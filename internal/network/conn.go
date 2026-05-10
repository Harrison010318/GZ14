package network

import (
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
)

// ConnState 连接状态
type ConnState int32

const (
	StateInit    ConnState = 0
	StateAuth    ConnState = 1 // 已鉴权
	StateInScene ConnState = 2 // 已在场景中
	StateClosed  ConnState = 3
)

// Conn TCP 连接封装
type Conn struct {
	id         int64
	rawConn    net.Conn
	state      atomic.Int32
	codec      *Codec
	readBuf    []byte // 粘包处理剩余数据

	readCh     chan *Packet
	writeCh    chan *Packet
	closeCh    chan struct{}
	closeOnce  sync.Once

	sessionID  string   // 登录后绑定
	roleID     int64    // 进入场景后绑定

	lastHeartbeat atomic.Int64 // 最后一次心跳时间戳（UnixNano）

	onPacket   func(c *Conn, pkt *Packet)
	onClose    func(c *Conn)

	wg         sync.WaitGroup
}

var connIDCounter atomic.Int64

func NewConn(rawConn net.Conn, onPacket func(*Conn, *Packet), onClose func(*Conn)) *Conn {
	c := &Conn{
		id:       connIDCounter.Add(1),
		rawConn:  rawConn,
		codec:    &Codec{},
		readBuf:  make([]byte, 0),
		readCh:   make(chan *Packet, 64),
		writeCh:  make(chan *Packet, 128),
		closeCh:  make(chan struct{}),
		onPacket: onPacket,
		onClose:  onClose,
	}
	c.state.Store(int32(StateInit))
	c.lastHeartbeat.Store(time.Now().UnixNano())

	c.wg.Add(2)
	go c.readLoop()
	go c.writeLoop()

	return c
}

func (c *Conn) ID() int64 {
	return c.id
}

func (c *Conn) State() ConnState {
	return ConnState(c.state.Load())
}

func (c *Conn) SetState(s ConnState) {
	c.state.Store(int32(s))
}

func (c *Conn) RemoteAddr() string {
	return c.rawConn.RemoteAddr().String()
}

func (c *Conn) SessionID() string {
	return c.sessionID
}

func (c *Conn) SetSessionID(sid string) {
	c.sessionID = sid
}

func (c *Conn) RoleID() int64 {
	return c.roleID
}

// Codec 返回连接关联的编解码器
func (c *Conn) Codec() *Codec {
	return c.codec
}

func (c *Conn) SetRoleID(rid int64) {
	c.roleID = rid
}

// UpdateHeartbeat 更新心跳时间戳
func (c *Conn) UpdateHeartbeat() {
	c.lastHeartbeat.Store(time.Now().UnixNano())
}

// HeartbeatElapsed 返回距离最后一次心跳的时间
func (c *Conn) HeartbeatElapsed() time.Duration {
	return time.Since(time.Unix(0, c.lastHeartbeat.Load()))
}

// SendPacket 发送一个 Packet（线程安全）
func (c *Conn) SendPacket(pkt *Packet) bool {
	select {
	case c.writeCh <- pkt:
		return true
	case <-c.closeCh:
		return false
	}
}

// SendMessage 编码并发送一个 protobuf 消息（线程安全）
func (c *Conn) SendMessage(msgID MsgID, msg proto.Message, seq uint32, errCode uint16) bool {
	return c.SendPacket(c.codec.EncodeToPacket(msgID, msg, seq, errCode))
}

// Close 安全关闭连接
func (c *Conn) Close() {
	c.closeOnce.Do(func() {
		c.state.Store(int32(StateClosed))
		close(c.closeCh)
		c.rawConn.SetReadDeadline(time.Now()) // 唤醒 readLoop
	})
	c.wg.Wait()
	c.rawConn.Close()
	if c.onClose != nil {
		c.onClose(c)
	}
}

// closeAsync 异步关闭连接（在 readLoop/writeLoop 内部调用避免死锁）
func (c *Conn) closeAsync() {
	go c.Close()
}

// readLoop 读取协程
func (c *Conn) readLoop() {
	defer c.wg.Done()

	for {
		pkt, remain, err := PacketReader(c.rawConn, c.readBuf)
		if err != nil {
			if c.state.Load() == int32(StateClosed) {
				return
			}
			log.Printf("[Conn:%d] read error: %v", c.id, err)
			c.closeAsync()
			return
		}
		c.readBuf = remain

		if c.onPacket != nil {
			c.onPacket(c, pkt)
		}

		select {
		case c.readCh <- pkt:
		case <-c.closeCh:
			return
		}
	}
}

// writeLoop 写入协程
func (c *Conn) writeLoop() {
	defer c.wg.Done()

	for {
		select {
		case pkt := <-c.writeCh:
			data := pkt.Marshal()
			if _, err := c.rawConn.Write(data); err != nil {
				log.Printf("[Conn:%d] write error: %v", c.id, err)
				c.closeAsync()
				return
			}
		case <-c.closeCh:
			return
		}
	}
}

// ReadPacket 从 readCh 获取一个 Packet（非阻塞）
func (c *Conn) ReadPacket() *Packet {
	select {
	case pkt := <-c.readCh:
		return pkt
	default:
		return nil
	}
}

// ReadPacketBlock 阻塞读取一个 Packet
func (c *Conn) ReadPacketBlock() (*Packet, error) {
	select {
	case pkt := <-c.readCh:
		return pkt, nil
	case <-c.closeCh:
		return nil, net.ErrClosed
	}
}
