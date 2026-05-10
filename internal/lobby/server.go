package lobby

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
	"time"

	"gz14/internal/db"
	"gz14/internal/network"
	"gz14/internal/protocol"
)

const (
	heartbeatCheckInterval = 5 * time.Second
	heartbeatTimeout       = 30 * time.Second
)

// Server Lobby 服务
type Server struct {
	*network.Server
	manager  *db.Manager
	sceneAPI SceneAPI

	seqCounter uint32
	seqMutex   sync.Mutex

	stopCh chan struct{}
}

// SceneAPI 场景服务接口（避免循环依赖）
type SceneAPI interface {
	PlayerEnterScene(roleID int64, mapID int, x, y float64)
	PlayerLeaveScene(roleID int64, mapID int)
	HandleMove(roleID int64, x, y float64)
}

func NewServer(addr string, manager *db.Manager) *Server {
	s := &Server{
		Server:  network.NewServer(addr),
		manager: manager,
		stopCh:  make(chan struct{}),
	}

	// 注册消息处理器
	s.Handle(network.MsgRegisterReq, s.handleRegister)
	s.Handle(network.MsgLoginReq, s.handleLogin)
	s.Handle(network.MsgCreateRoleReq, s.handleCreateRole)
	s.Handle(network.MsgLogoutReq, s.handleLogout)
	s.Handle(network.MsgHeartbeatReq, s.handleHeartbeat)
	s.Handle(network.MsgEnterSceneReq, s.handleEnterScene)
	s.Handle(network.MsgMoveReq, s.handleMove)
	s.Handle(network.MsgReconnectReq, s.handleReconnect)

	return s
}

// SetSceneAPI 设置场景服务接口（创建后注入，避免循环依赖）
func (s *Server) SetSceneAPI(sa SceneAPI) {
	s.sceneAPI = sa
}

// Start 重写 Start，添加心跳检测协程
func (s *Server) Start() error {
	if err := s.Server.Start(); err != nil {
		return err
	}
	go s.heartbeatCheckLoop()
	return nil
}

// Stop 重写 Stop，清理心跳检测
func (s *Server) Stop() {
	close(s.stopCh)
	s.Server.Stop()
}

func (s *Server) nextSeq() uint32 {
	s.seqMutex.Lock()
	defer s.seqMutex.Unlock()
	s.seqCounter++
	return s.seqCounter
}

func generateToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ========== 心跳超时检测 ==========

// heartbeatCheckLoop 定期检查所有连接的心跳超时
func (s *Server) heartbeatCheckLoop() {
	ticker := time.NewTicker(heartbeatCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkHeartbeatTimeouts()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Server) checkHeartbeatTimeouts() {
	s.Server.RangeConns(func(c *network.Conn) bool {
		state := c.State()
		// 只检查已鉴权或在场景中的连接
		if state != network.StateAuth && state != network.StateInScene {
			return true
		}
		if c.HeartbeatElapsed() > heartbeatTimeout {
			roleID := c.RoleID()
			log.Printf("[Lobby] heartbeat timeout: conn=%d role=%d elapsed=%v",
				c.ID(), roleID, c.HeartbeatElapsed())

			// 如果在场景中，需要清理场景状态
			if roleID > 0 && state == network.StateInScene {
				s.manager.DelOnline(context.Background(), roleID)
				x, y, mapID, _ := s.manager.GetPosition(context.Background(), roleID)
				if mapID > 0 {
					s.manager.UpdateRolePosition(context.Background(), roleID, x, y, mapID)
				}
				s.manager.DelPosition(context.Background(), roleID)
				s.sceneAPI.PlayerLeaveScene(roleID, 0)
			}
			// 清理 Session
			if sid := c.SessionID(); sid != "" {
				s.manager.DelSession(context.Background(), sid)
			}
			c.Close()
		}
		return true
	})
}

// ========== Handler: Register ==========

func (s *Server) handleRegister(c *network.Conn, pkt *network.Packet) {
	req := &protocol.RegisterReq{}
	if err := c.Codec().Decode(pkt.Body, req); err != nil {
		log.Printf("[Lobby] decode RegisterReq error: %v", err)
		return
	}

	log.Printf("[Lobby] register: username=%s", req.Username)

	// 检查账号是否存在
	existing, _ := s.manager.GetAccountByUsername(context.Background(), req.Username)
	if existing != nil {
		c.SendMessage(network.MsgRegisterRsp, &protocol.RegisterRsp{
			ErrCode: protocol.ErrCode_ERR_ACCOUNT_EXIST,
		}, pkt.Sequence, 0)
		return
	}

	acc, err := s.manager.CreateAccount(context.Background(), req.Username, req.Password)
	if err != nil {
		log.Printf("[Lobby] create account error: %v", err)
		c.SendMessage(network.MsgRegisterRsp, &protocol.RegisterRsp{
			ErrCode: protocol.ErrCode_ERR_UNKNOWN,
		}, pkt.Sequence, 0)
		return
	}

	log.Printf("[Lobby] register success: account_id=%d", acc.ID)
	c.SendMessage(network.MsgRegisterRsp, &protocol.RegisterRsp{
		ErrCode:   protocol.ErrCode_ERR_OK,
		AccountId: acc.ID,
	}, pkt.Sequence, 0)
}

// ========== Handler: Login ==========

func (s *Server) handleLogin(c *network.Conn, pkt *network.Packet) {
	req := &protocol.LoginReq{}
	if err := c.Codec().Decode(pkt.Body, req); err != nil {
		return
	}

	log.Printf("[Lobby] login: username=%s", req.Username)

	// 查询账号
	acc, err := s.manager.GetAccountByUsername(context.Background(), req.Username)
	if err != nil {
		c.SendMessage(network.MsgLoginRsp, &protocol.LoginRsp{
			ErrCode: protocol.ErrCode_ERR_ACCOUNT_NOT_FOUND,
		}, pkt.Sequence, 0)
		return
	}

	// 验证密码
	if !s.manager.VerifyPassword(acc.Password, req.Password) {
		c.SendMessage(network.MsgLoginRsp, &protocol.LoginRsp{
			ErrCode: protocol.ErrCode_ERR_WRONG_PASSWORD,
		}, pkt.Sequence, 0)
		return
	}

	// 生成 Token
	token := generateToken()
	if err := s.manager.SetSession(context.Background(), token, acc.ID); err != nil {
		log.Printf("[Lobby] set session error: %v", err)
		c.SendMessage(network.MsgLoginRsp, &protocol.LoginRsp{
			ErrCode: protocol.ErrCode_ERR_UNKNOWN,
		}, pkt.Sequence, 0)
		return
	}

	c.SetState(network.StateAuth)
	c.SetSessionID(token)
	c.UpdateHeartbeat()

	// 查询角色列表
	roles, _ := s.manager.GetRolesByAccount(context.Background(), acc.ID)
	playerInfos := make([]*protocol.PlayerInfo, 0, len(roles))
	for i := range roles {
		playerInfos = append(playerInfos, &protocol.PlayerInfo{
			Id:    roles[i].ID,
			Name:  roles[i].Name,
			Level: int32(roles[i].Level),
			MapId: int32(roles[i].MapID),
			Position: &protocol.Vec2{
				X: roles[i].PosX,
				Y: roles[i].PosY,
			},
		})
	}

	log.Printf("[Lobby] login success: account_id=%d, roles=%d", acc.ID, len(roles))

	rsp := &protocol.LoginRsp{
		ErrCode: protocol.ErrCode_ERR_OK,
		Token:   token,
		Roles:   playerInfos,
	}
	c.SendMessage(network.MsgLoginRsp, rsp, pkt.Sequence, 0)
}

// ========== Handler: CreateRole ==========

func (s *Server) handleCreateRole(c *network.Conn, pkt *network.Packet) {
	req := &protocol.CreateRoleReq{}
	if err := c.Codec().Decode(pkt.Body, req); err != nil {
		return
	}

	// 验证 Session
	accountID, err := s.manager.GetSession(context.Background(), req.Token)
	if err != nil {
		c.SendMessage(network.MsgCreateRoleRsp, &protocol.CreateRoleRsp{
			ErrCode: protocol.ErrCode_ERR_SESSION_INVALID,
		}, pkt.Sequence, 0)
		return
	}

	// 检查名字是否重复
	exist, _ := s.manager.IsRoleNameExist(context.Background(), req.Name)
	if exist {
		c.SendMessage(network.MsgCreateRoleRsp, &protocol.CreateRoleRsp{
			ErrCode: protocol.ErrCode_ERR_ROLE_NAME_EXIST,
		}, pkt.Sequence, 0)
		return
	}

	role, err := s.manager.CreateRole(context.Background(), accountID, req.Name)
	if err != nil {
		errCode := protocol.ErrCode_ERR_UNKNOWN
		if err == db.ErrRoleFull {
			errCode = protocol.ErrCode_ERR_ROLE_FULL
		}
		c.SendMessage(network.MsgCreateRoleRsp, &protocol.CreateRoleRsp{
			ErrCode: errCode,
		}, pkt.Sequence, 0)
		return
	}

	log.Printf("[Lobby] create role success: role_id=%d, name=%s", role.ID, role.Name)

	c.SendMessage(network.MsgCreateRoleRsp, &protocol.CreateRoleRsp{
		ErrCode: protocol.ErrCode_ERR_OK,
		Role: &protocol.PlayerInfo{
			Id:    role.ID,
			Name:  role.Name,
			Level: int32(role.Level),
			MapId: int32(role.MapID),
			Position: &protocol.Vec2{
				X: role.PosX,
				Y: role.PosY,
			},
		},
	}, pkt.Sequence, 0)
}

// ========== Handler: EnterScene ==========

func (s *Server) handleEnterScene(c *network.Conn, pkt *network.Packet) {
	req := &protocol.EnterSceneReq{}
	if err := c.Codec().Decode(pkt.Body, req); err != nil {
		return
	}

	// 验证 Session（用 token 而非 role_id 来鉴权）
	accountID, err := s.manager.GetSession(context.Background(), req.Token)
	if err != nil {
		c.SendMessage(network.MsgEnterSceneRsp, &protocol.EnterSceneRsp{
			ErrCode: protocol.ErrCode_ERR_SESSION_INVALID,
		}, pkt.Sequence, 0)
		return
	}

	// 验证角色属于该账号
	role, err := s.manager.GetRoleByID(context.Background(), req.RoleId)
	if err != nil || role.AccountID != accountID {
		c.SendMessage(network.MsgEnterSceneRsp, &protocol.EnterSceneRsp{
			ErrCode: protocol.ErrCode_ERR_ROLE_NOT_FOUND,
		}, pkt.Sequence, 0)
		return
	}

	// 标记连接绑定角色
	c.SetState(network.StateInScene)
	c.SetRoleID(role.ID)
	c.UpdateHeartbeat()

	// 设置在线状态
	s.manager.SetOnline(context.Background(), role.ID, c.RemoteAddr())
	// 缓存初始位置
	s.manager.SetPosition(context.Background(), role.ID, role.PosX, role.PosY, role.MapID)

	// 通知 Scene 服务
	s.sceneAPI.PlayerEnterScene(role.ID, role.MapID, role.PosX, role.PosY)

	rsp := &protocol.EnterSceneRsp{
		ErrCode:    protocol.ErrCode_ERR_OK,
		MapId:      int32(role.MapID),
		Position:   &protocol.Vec2{X: role.PosX, Y: role.PosY},
		AoiPlayers: make([]*protocol.PlayerInfo, 0),
	}
	c.SendMessage(network.MsgEnterSceneRsp, rsp, pkt.Sequence, 0)
}

// ========== Handler: Reconnect ==========

func (s *Server) handleReconnect(c *network.Conn, pkt *network.Packet) {
	req := &protocol.ReconnectReq{}
	if err := c.Codec().Decode(pkt.Body, req); err != nil {
		return
	}

	log.Printf("[Lobby] reconnect: role=%d", req.RoleId)

	// 验证 Token 是否有效
	accountID, err := s.manager.GetSession(context.Background(), req.Token)
	if err != nil {
		c.SendMessage(network.MsgReconnectRsp, &protocol.ReconnectRsp{
			ErrCode: protocol.ErrCode_ERR_SESSION_INVALID,
		}, pkt.Sequence, 0)
		return
	}

	// 验证角色属于该账号
	role, err := s.manager.GetRoleByID(context.Background(), req.RoleId)
	if err != nil || role.AccountID != accountID {
		c.SendMessage(network.MsgReconnectRsp, &protocol.ReconnectRsp{
			ErrCode: protocol.ErrCode_ERR_ROLE_NOT_FOUND,
		}, pkt.Sequence, 0)
		return
	}

	// Token 续期：重新设置 Session（刷新 TTL）
	s.manager.SetSession(context.Background(), req.Token, accountID)

	// 重新绑定连接
	c.SetState(network.StateInScene)
	c.SetSessionID(req.Token)
	c.SetRoleID(role.ID)
	c.UpdateHeartbeat()

	// 重新设置在线状态
	s.manager.SetOnline(context.Background(), role.ID, c.RemoteAddr())

	// 从缓存获取最新位置，如果没有则用 DB 中的位置
	x, y, mapID, err := s.manager.GetPosition(context.Background(), role.ID)
	if err != nil {
		x, y, mapID = role.PosX, role.PosY, role.MapID
	}

	// 重新进入场景（Scene 服务会处理 AOI 恢复）
	s.sceneAPI.PlayerEnterScene(role.ID, mapID, x, y)

	log.Printf("[Lobby] reconnect success: role=%d account=%d pos=(%.1f,%.1f) map=%d",
		role.ID, accountID, x, y, mapID)

	c.SendMessage(network.MsgReconnectRsp, &protocol.ReconnectRsp{
		ErrCode:  protocol.ErrCode_ERR_OK,
		MapId:    int32(mapID),
		Position: &protocol.Vec2{X: x, Y: y},
	}, pkt.Sequence, 0)
}

// ========== Handler: Logout ==========

func (s *Server) handleLogout(c *network.Conn, pkt *network.Packet) {
	req := &protocol.LogoutReq{}
	if err := c.Codec().Decode(pkt.Body, req); err != nil {
		return
	}

	accountID, err := s.manager.GetSession(context.Background(), req.Token)
	if err != nil {
		c.SendMessage(network.MsgLogoutRsp, &protocol.LogoutRsp{
			ErrCode: protocol.ErrCode_ERR_SESSION_INVALID,
		}, pkt.Sequence, 0)
		return
	}

	roleID := c.RoleID()

	// 清理在线状态
	if roleID > 0 {
		s.manager.DelOnline(context.Background(), roleID)
		x, y, mapID, _ := s.manager.GetPosition(context.Background(), roleID)
		// 位置写回 MySQL
		if mapID > 0 {
			s.manager.UpdateRolePosition(context.Background(), roleID, x, y, mapID)
		}
		s.manager.DelPosition(context.Background(), roleID)
		s.sceneAPI.PlayerLeaveScene(roleID, 0)
	}

	// 清理 Session
	s.manager.DelSession(context.Background(), req.Token)

	c.SetState(network.StateClosed)
	c.SetSessionID("")
	c.SetRoleID(0)

	log.Printf("[Lobby] logout: account_id=%d", accountID)

	c.SendMessage(network.MsgLogoutRsp, &protocol.LogoutRsp{
		ErrCode: protocol.ErrCode_ERR_OK,
	}, pkt.Sequence, 0)

	// 延迟关闭连接，让响应发送完成
	time.AfterFunc(100*time.Millisecond, func() {
		c.Close()
	})
}

// ========== Handler: Heartbeat ==========

func (s *Server) handleMove(c *network.Conn, pkt *network.Packet) {
	req := &protocol.MoveReq{}
	if err := c.Codec().Decode(pkt.Body, req); err != nil {
		return
	}
	if s.sceneAPI != nil {
		newX := req.Position.GetX()
		newY := req.Position.GetY()
		s.sceneAPI.HandleMove(req.RoleId, newX, newY)
	}
}

func (s *Server) handleHeartbeat(c *network.Conn, pkt *network.Packet) {
	c.UpdateHeartbeat()
	c.SendMessage(network.MsgHeartbeatRsp, &protocol.HeartbeatRsp{}, pkt.Sequence, 0)
}
