package network

import (
	"log"
	"net"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"
)

// HandlerFunc 消息处理函数
type HandlerFunc func(c *Conn, pkt *Packet)

// Server TCP 服务器基础框架
type Server struct {
	addr      string
	listener  net.Listener
	running   atomic.Bool
	handlers  map[MsgID]HandlerFunc
	conns     map[int64]*Conn
	connMutex sync.RWMutex
	wg        sync.WaitGroup
}

func NewServer(addr string) *Server {
	return &Server{
		addr:     addr,
		handlers: make(map[MsgID]HandlerFunc),
		conns:    make(map[int64]*Conn),
	}
}

func (s *Server) Addr() string {
	return s.addr
}

// Handle 注册消息处理器
func (s *Server) Handle(msgID MsgID, handler HandlerFunc) {
	s.handlers[msgID] = handler
}

// Start 启动服务器
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = ln
	s.running.Store(true)

	log.Printf("[Server] listening on %s", s.addr)

	s.wg.Add(1)
	go s.acceptLoop()
	return nil
}

// Stop 停止服务器
func (s *Server) Stop() {
	s.running.Store(false)
	if s.listener != nil {
		s.listener.Close()
	}

	s.connMutex.RLock()
	for _, conn := range s.conns {
		conn.Close()
	}
	s.connMutex.RUnlock()

	s.wg.Wait()
	log.Printf("[Server] %s stopped", s.addr)
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for s.running.Load() {
		rawConn, err := s.listener.Accept()
		if err != nil {
			if s.running.Load() {
				log.Printf("[Server] accept error: %v", err)
			}
			return
		}

		conn := NewConn(rawConn, s.dispatchPacket, func(c *Conn) { s.removeConn(c) })
		s.addConn(conn)
		log.Printf("[Server] new connection: id=%d addr=%s", conn.ID(), conn.RemoteAddr())
	}
}

func (s *Server) addConn(c *Conn) {
	s.connMutex.Lock()
	s.conns[c.ID()] = c
	s.connMutex.Unlock()
}

func (s *Server) removeConn(c *Conn) {
	s.connMutex.Lock()
	delete(s.conns, c.ID())
	s.connMutex.Unlock()
}

// dispatchPacket 根据 MsgID 分发到对应的处理器
func (s *Server) dispatchPacket(c *Conn, pkt *Packet) {
	handler, ok := s.handlers[pkt.MsgID]
	if !ok {
		log.Printf("[Server] no handler for msg %s (0x%04X)", pkt.MsgID, uint16(pkt.MsgID))
		return
	}
	handler(c, pkt)
}

// Broadcast 向所有连接广播消息
func (s *Server) Broadcast(msgID MsgID, msg proto.Message) int {
	count := 0
	s.connMutex.RLock()
	for _, conn := range s.conns {
		if conn.SendMessage(msgID, msg, 0, 0) {
			count++
		}
	}
	s.connMutex.RUnlock()
	return count
}

// RangeConns 遍历所有连接，fn 返回 false 停止遍历
func (s *Server) RangeConns(fn func(c *Conn) bool) {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()
	for _, conn := range s.conns {
		if !fn(conn) {
			break
		}
	}
}

// ConnCount 获取当前连接数
func (s *Server) ConnCount() int {
	s.connMutex.RLock()
	n := len(s.conns)
	s.connMutex.RUnlock()
	return n
}

// GetConn 根据角色 ID 查找连接（遍历，小规模可用）
func (s *Server) GetConnByRoleID(roleID int64) *Conn {
	s.connMutex.RLock()
	defer s.connMutex.RUnlock()
	for _, conn := range s.conns {
		if conn.RoleID() == roleID {
			return conn
		}
	}
	return nil
}
