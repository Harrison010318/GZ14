package bot

import (
	"fmt"
	"math/rand"
	"net"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"gz14/internal/network"
	"gz14/internal/protocol"

	"google.golang.org/protobuf/proto"
)

// ClientState 机器人状态
type ClientState int32

const (
	StateInit    ClientState = 0
	StateLogin   ClientState = 1
	StateInScene ClientState = 2
	StateDone    ClientState = 3
)

// Client 模拟游戏客户端的机器人
type Client struct {
	ID        int
	state     atomic.Int32
	conn      *network.Conn
	Username  string
	Password  string
	serverAddr string

	Token  string
	RoleID int64
	PosX   float64
	PosY   float64
	codec  *network.Codec

	// 统计字段
	MoveSent       atomic.Int64
	BroadcastRecv  atomic.Int64
	AOIEnterRecv   atomic.Int64
	AOILeaveRecv   atomic.Int64
	LoginLatency   time.Duration
	EnterLatency   time.Duration
	LoginErr       error
	EnterErr       error

	lastSeq  int32
	closeOnce sync.Once
	running   atomic.Bool
	stopCh    chan struct{}
}

func NewClient(id int, serverAddr string) *Client {
	return &Client{
		ID:         id,
		serverAddr: serverAddr,
		Username:   fmt.Sprintf("bot_%d", id),
		Password:   "pass123",
		codec:      &network.Codec{},
		PosX:       100 + rand.Float64()*800,
		PosY:       100 + rand.Float64()*800,
		stopCh:     make(chan struct{}),
	}
}

func (c *Client) State() ClientState {
	return ClientState(c.state.Load())
}

func (c *Client) IsRunning() bool {
	return c.running.Load()
}

// cleanup 清理资源，确保 recvLoop 退出
func (c *Client) cleanup() {
	c.closeOnce.Do(func() {
		close(c.stopCh)
	})
	if c.conn != nil {
		c.conn.Close()
	}
}

// Run 执行完整生命周期
func (c *Client) Run() {
	// 1. Connect
	rawConn, err := net.DialTimeout("tcp", c.serverAddr, 5*time.Second)
	if err != nil {
		c.LoginErr = fmt.Errorf("connect failed: %w", err)
		return
	}

	recvCh := make(chan *network.Packet, 256)
	c.conn = network.NewConn(rawConn, nil, nil)

	// 启动后台接收协程（持续消费 recvCh 中的广播消息）
	go c.recvLoop(recvCh)

	// 2. Register + Login（同步等待）
	if err := c.doLogin(); err != nil {
		c.cleanup()
		return
	}

	// 3. Create role (best effort)
	c.doCreateRole()

	// 4. Enter scene（同步等待）
	if err := c.doEnterScene(); err != nil {
		c.cleanup()
		return
	}

	c.running.Store(true)

	// 启动后台协程持续复制 conn.readCh → recvCh
	go c.forwardPackets(recvCh)
}

func (c *Client) sendMsg(msgID network.MsgID, msg proto.Message) {
	body := c.codec.Encode(msg)
	pkt := &network.Packet{MsgID: msgID, Body: body}
	c.conn.SendPacket(pkt)
}

func (c *Client) doLogin() error {
	start := time.Now()

	// 先发注册（可能失败，忽略）
	c.sendMsg(network.MsgRegisterReq, &protocol.RegisterReq{
		Username: c.Username, Password: c.Password,
	})

	// 再发登录
	c.sendMsg(network.MsgLoginReq, &protocol.LoginReq{
		Username: c.Username, Password: c.Password,
	})

	// 等待 LoginRsp
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pkt := c.conn.ReadPacket()
		if pkt == nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if pkt.MsgID == network.MsgLoginRsp {
			rsp := &protocol.LoginRsp{}
			if err := c.codec.Decode(pkt.Body, rsp); err != nil {
				c.LoginErr = err
				return err
			}
			if rsp.ErrCode != protocol.ErrCode_ERR_OK {
				c.LoginErr = fmt.Errorf("login failed: %v", rsp.ErrCode)
				return c.LoginErr
			}
			c.Token = rsp.Token
			if len(rsp.Roles) > 0 {
				c.RoleID = rsp.Roles[0].Id
			}
			c.LoginLatency = time.Since(start)
			c.state.Store(int32(StateLogin))
			return nil
		}
	}
	c.LoginErr = fmt.Errorf("login timeout")
	return c.LoginErr
}

func (c *Client) doCreateRole() {
	if c.RoleID > 0 {
		return
	}
	c.sendMsg(network.MsgCreateRoleReq, &protocol.CreateRoleReq{
		Token: c.Token,
		Name:  fmt.Sprintf("hero_%d", c.ID),
	})

	// 等待 CreateRoleRsp
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pkt := c.conn.ReadPacket()
		if pkt == nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if pkt.MsgID == network.MsgCreateRoleRsp {
			rsp := &protocol.CreateRoleRsp{}
			if err := c.codec.Decode(pkt.Body, rsp); err != nil {
				return
			}
			if rsp.Role != nil {
				c.RoleID = rsp.Role.Id
				c.PosX = rsp.Role.Position.GetX()
				c.PosY = rsp.Role.Position.GetY()
			}
			return
		}
		// 可能先收到其他包（如 RegisterRsp），忽略
	}
}

func (c *Client) doEnterScene() error {
	if c.RoleID == 0 {
		c.EnterErr = fmt.Errorf("no role id")
		return c.EnterErr
	}

	start := time.Now()
	c.sendMsg(network.MsgEnterSceneReq, &protocol.EnterSceneReq{
		Token: c.Token, RoleId: c.RoleID,
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pkt := c.conn.ReadPacket()
		if pkt == nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if pkt.MsgID == network.MsgEnterSceneRsp {
			rsp := &protocol.EnterSceneRsp{}
			if err := c.codec.Decode(pkt.Body, rsp); err != nil {
				c.EnterErr = err
				return err
			}
			if rsp.ErrCode != protocol.ErrCode_ERR_OK {
				c.EnterErr = fmt.Errorf("enter scene failed: %v", rsp.ErrCode)
				return c.EnterErr
			}
			if rsp.Position != nil {
				c.PosX = rsp.Position.X
				c.PosY = rsp.Position.Y
			}
			c.EnterLatency = time.Since(start)
			c.state.Store(int32(StateInScene))
			return nil
		}
	}
	c.EnterErr = fmt.Errorf("enter scene timeout")
	return c.EnterErr
}

// forwardPackets 将 conn 收到但不被同步方法消费的包转发到 recvCh
func (c *Client) forwardPackets(recvCh chan<- *network.Packet) {
	for {
		pkt, err := c.conn.ReadPacketBlock()
		if err != nil || pkt == nil {
			return
		}
		select {
		case recvCh <- pkt:
		default:
		}
	}
}

func (c *Client) recvLoop(recvCh <-chan *network.Packet) {
	hbTicker := time.NewTicker(5 * time.Second)
	defer hbTicker.Stop()

	for {
		select {
		case pkt := <-recvCh:
			c.handlePacket(pkt)
		case <-hbTicker.C:
			c.sendMsg(network.MsgHeartbeatReq, &protocol.HeartbeatReq{})
		case <-c.stopCh:
			return
		}
	}
}

func (c *Client) handlePacket(pkt *network.Packet) {
	switch pkt.MsgID {
	case network.MsgMoveBroadcast:
		c.BroadcastRecv.Add(1)
	case network.MsgAOIEnter:
		c.AOIEnterRecv.Add(1)
	case network.MsgAOILeave:
		c.AOILeaveRecv.Add(1)
	}
}

// DoReconnect 执行断线重连
func (c *Client) DoReconnect() error {
	start := time.Now()

	// 发送重连请求
	c.sendMsg(network.MsgReconnectReq, &protocol.ReconnectReq{
		Token: c.Token, RoleId: c.RoleID,
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		pkt := c.conn.ReadPacket()
		if pkt == nil {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		if pkt.MsgID == network.MsgReconnectRsp {
			rsp := &protocol.ReconnectRsp{}
			if err := c.codec.Decode(pkt.Body, rsp); err != nil {
				return fmt.Errorf("decode reconnect rsp error: %w", err)
			}
			if rsp.ErrCode != protocol.ErrCode_ERR_OK {
				return fmt.Errorf("reconnect failed: %v", rsp.ErrCode)
			}
			if rsp.Position != nil {
				c.PosX = rsp.Position.X
				c.PosY = rsp.Position.Y
			}
			c.state.Store(int32(StateInScene))
			c.running.Store(true)
			log.Printf("[Bot:%d] reconnect success in %v (map=%d)", c.ID, time.Since(start), rsp.MapId)
			return nil
		}
	}
	return fmt.Errorf("reconnect timeout")
}

// CloseConn 关闭当前连接
func (c *Client) CloseConn() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// SetState 设置 bot 状态
func (c *Client) SetState(s ClientState) {
	c.state.Store(int32(s))
}

// ReplaceConn 替换连接（断线重连时使用）
func (c *Client) ReplaceConn(rawConn net.Conn) {
	// 关闭旧连接
	if c.conn != nil {
		c.conn.Close()
	}
	// 创建新连接
	c.conn = network.NewConn(rawConn, nil, nil)
}

func (c *Client) SendMove() {
	dx := (rand.Float64() - 0.5) * 20
	dy := (rand.Float64() - 0.5) * 20
	nx := clamp(c.PosX+dx, 0, 1000)
	ny := clamp(c.PosY+dy, 0, 1000)

	c.sendMsg(network.MsgMoveReq, &protocol.MoveReq{
		RoleId: c.RoleID,
		Position: &protocol.Vec2{X: nx, Y: ny},
	})
	c.PosX = nx
	c.PosY = ny
	c.MoveSent.Add(1)
}

// Logout 主动登出
func (c *Client) Logout() {
	c.sendMsg(network.MsgLogoutReq, &protocol.LogoutReq{Token: c.Token})
	time.Sleep(100 * time.Millisecond)
	c.state.Store(int32(StateDone))
	c.running.Store(false)
	c.closeOnce.Do(func() {
		close(c.stopCh)
	})
	if c.conn != nil {
		c.conn.Close()
	}
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
