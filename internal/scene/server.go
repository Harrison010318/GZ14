package scene

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"gz14/internal/db"
	"gz14/internal/network"
	"gz14/internal/protocol"
)

// Server Scene 服务
type Server struct {
	*network.Server
	manager     *db.Manager
	sceneMgr    *SceneManager
	lobby       LobbyAPI
	moveLimiter *MoveLimiter

	seqCounter uint32
	seqMutex   sync.Mutex
	// 角色ID → Conn 映射
	roleConns map[int64]*network.Conn
	rcMutex   sync.RWMutex
}

// LobbyAPI 避免循环依赖的接口
type LobbyAPI interface {
	GetConnByRoleID(roleID int64) *network.Conn
}

// MoveLimiter 移动频率限制
type MoveLimiter struct {
	lastMove map[int64]time.Time
	mutex    sync.Mutex
	interval time.Duration
}

func NewMoveLimiter(interval time.Duration) *MoveLimiter {
	return &MoveLimiter{
		lastMove: make(map[int64]time.Time),
		interval: interval,
	}
}

func (ml *MoveLimiter) Allow(roleID int64) bool {
	ml.mutex.Lock()
	defer ml.mutex.Unlock()
	last, ok := ml.lastMove[roleID]
	now := time.Now()
	if !ok || now.Sub(last) >= ml.interval {
		ml.lastMove[roleID] = now
		return true
	}
	return false
}

func (ml *MoveLimiter) Remove(roleID int64) {
	ml.mutex.Lock()
	delete(ml.lastMove, roleID)
	ml.mutex.Unlock()
}

func NewServer(addr string, manager *db.Manager, sceneMgr *SceneManager) *Server {
	s := &Server{
		Server:      network.NewServer(addr),
		manager:     manager,
		sceneMgr:    sceneMgr,
		moveLimiter: NewMoveLimiter(50 * time.Millisecond), // 20次/秒
		roleConns:   make(map[int64]*network.Conn),
	}

	// 注册处理器 - Scene 服务处理客户端的移动请求
	s.Handle(network.MsgMoveReq, s.handleMove)

	return s
}

// SetLobbyAPI 设置 Lobby 服务接口（创建后注入，避免循环依赖）
func (s *Server) SetLobbyAPI(lobby LobbyAPI) {
	s.lobby = lobby
}

func (s *Server) nextSeq() uint32 {
	s.seqMutex.Lock()
	defer s.seqMutex.Unlock()
	s.seqCounter++
	return s.seqCounter
}

// ========== SceneAPI 实现（由 Lobby 调用）==========

// PlayerEnterScene 由 Lobby 通知 Scene：玩家进入场景
func (s *Server) PlayerEnterScene(roleID int64, mapID int, x, y float64) {
	scene := s.sceneMgr.GetOrCreateScene(mapID)

	// 获取角色名字
	role, err := s.manager.GetRoleByID(context.Background(), roleID)
	name := ""
	if err == nil {
		name = role.Name
	}

	// 添加到场景
	others := scene.AddPlayer(roleID, name, x, y)

	// 构建 AOIEnter 消息
	aoiPlayers := make([]*protocol.PlayerInfo, 0, len(others))
	enterIDs := make([]int64, 0, len(others))
	for _, other := range others {
		aoiPlayers = append(aoiPlayers, &protocol.PlayerInfo{
			Id:   other.ID,
			Name: other.Name,
			Position: &protocol.Vec2{
				X: other.Position.X,
				Y: other.Position.Y,
			},
		})
		enterIDs = append(enterIDs, other.ID)
	}

	// 通知新玩家：周围已有玩家
	if len(aoiPlayers) > 0 {
		conn := s.lobby.GetConnByRoleID(roleID)
		if conn != nil {
			conn.SendMessage(network.MsgAOIEnter, &protocol.AOIEnter{
				Players: aoiPlayers,
			}, s.nextSeq(), 0)
		}
	}

	// 通知旧玩家：新玩家进入他们的视野
	for _, otherID := range enterIDs {
		otherConn := s.lobby.GetConnByRoleID(otherID)
		if otherConn != nil {
			otherConn.SendMessage(network.MsgAOIEnter, &protocol.AOIEnter{
				Players: []*protocol.PlayerInfo{{
					Id:   roleID,
					Name: name,
					Position: &protocol.Vec2{
						X: x,
						Y: y,
					},
				}},
			}, s.nextSeq(), 0)
		}
	}

	// 注册角色连接映射
	if conn := s.lobby.GetConnByRoleID(roleID); conn != nil {
		s.rcMutex.Lock()
		s.roleConns[roleID] = conn
		s.rcMutex.Unlock()
	}

	log.Printf("[Scene] player enter scene: role=%d map=%d pos=(%.1f,%.1f) visible=%d",
		roleID, mapID, x, y, len(others))
}

// PlayerLeaveScene 由 Lobby 通知 Scene：玩家离开场景
func (s *Server) PlayerLeaveScene(roleID int64, mapID int) {
	scene := s.sceneMgr.GetScene(mapID)
	if scene == nil {
		return
	}

	// 获取实体位置后通知周围玩家
	entity := scene.em.Get(roleID)
	if entity != nil {
		visible := scene.GetVisiblePlayers(roleID, entity.Position.X, entity.Position.Y)
		leaveIDs := make([]int64, 0, len(visible))
		for _, other := range visible {
			leaveIDs = append(leaveIDs, other.ID)
		}

		// 通知周围玩家：该玩家离开
		for _, otherID := range leaveIDs {
			otherConn := s.lobby.GetConnByRoleID(otherID)
			if otherConn != nil {
				otherConn.SendMessage(network.MsgAOILeave, &protocol.AOILeave{
					RoleIds: []int64{roleID},
				}, s.nextSeq(), 0)
			}
		}
	}

	scene.RemovePlayer(roleID)
	s.moveLimiter.Remove(roleID)

	s.rcMutex.Lock()
	delete(s.roleConns, roleID)
	s.rcMutex.Unlock()

	log.Printf("[Scene] player leave scene: role=%d map=%d", roleID, mapID)
}

// ========== Move API（被 SceneAPI 接口调用和 TCP handler 调用）==========

// HandleMove 处理移动逻辑，由 Lobby 的 MoveReq handler 或 Scene 的 TCP handler 调用
func (s *Server) HandleMove(roleID int64, x, y float64) {
	scene := s.sceneMgr.FindSceneByRoleID(roleID)
	if scene == nil {
		return
	}

	newX := math.Max(0, math.Min(1000.0, x))
	newY := math.Max(0, math.Min(1000.0, y))

	if !s.moveLimiter.Allow(roleID) {
		return
	}

	aoiEnter, aoiLeave, err := scene.MovePlayer(roleID, newX, newY)
	if err != nil {
		return
	}

	for _, enter := range aoiEnter {
		if otherConn := s.getConn(enter.ID); otherConn != nil {
			otherConn.SendMessage(network.MsgAOIEnter, &protocol.AOIEnter{
				Players: []*protocol.PlayerInfo{{Id: roleID, Position: &protocol.Vec2{X: newX, Y: newY}}},
			}, s.nextSeq(), 0)
		}
		if currConn := s.getConn(roleID); currConn != nil {
			currConn.SendMessage(network.MsgAOIEnter, &protocol.AOIEnter{
				Players: []*protocol.PlayerInfo{{Id: enter.ID, Name: enter.Name, Position: &protocol.Vec2{X: enter.Position.X, Y: enter.Position.Y}}},
			}, s.nextSeq(), 0)
		}
	}

	for _, leave := range aoiLeave {
		if otherConn := s.getConn(leave.ID); otherConn != nil {
			otherConn.SendMessage(network.MsgAOILeave, &protocol.AOILeave{RoleIds: []int64{roleID}}, s.nextSeq(), 0)
		}
		if currConn := s.getConn(roleID); currConn != nil {
			currConn.SendMessage(network.MsgAOILeave, &protocol.AOILeave{RoleIds: []int64{leave.ID}}, s.nextSeq(), 0)
		}
	}

	visible := scene.GetVisiblePlayers(roleID, newX, newY)
	seq := s.nextSeq()
	for _, other := range visible {
		if otherConn := s.getConn(other.ID); otherConn != nil {
			otherConn.SendMessage(network.MsgMoveBroadcast, &protocol.MoveBroadcast{
				RoleId: roleID, Position: &protocol.Vec2{X: newX, Y: newY}, Sequence: int32(seq),
			}, 0, 0)
		}
	}

	go func() {
		s.manager.SetPosition(context.Background(), roleID, newX, newY, scene.ID)
	}()
}

// ========== Handler: Move（TCP 入口）==========

func (s *Server) handleMove(c *network.Conn, pkt *network.Packet) {
	req := &protocol.MoveReq{}
	if err := c.Codec().Decode(pkt.Body, req); err != nil {
		return
	}
	s.HandleMove(req.RoleId, req.Position.GetX(), req.Position.GetY())
}

func (s *Server) getConn(roleID int64) *network.Conn {
	// 优先从本地映射查找
	s.rcMutex.RLock()
	conn, ok := s.roleConns[roleID]
	s.rcMutex.RUnlock()
	if ok {
		return conn
	}
	// 回退到 Lobby 查找
	return s.lobby.GetConnByRoleID(roleID)
}
