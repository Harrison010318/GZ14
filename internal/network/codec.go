package network

import (
	"gz14/internal/protocol"

	"google.golang.org/protobuf/proto"
)

// Codec Protobuf 序列化/反序列化封装
type Codec struct{}

// Encode 将 proto.Message 编码为 Packet 的 Body 字节
func (c *Codec) Encode(msg proto.Message) []byte {
	data, err := proto.Marshal(msg)
	if err != nil {
		return nil
	}
	return data
}

// EncodeToPacket 编码为完整的 Packet
func (c *Codec) EncodeToPacket(msgID MsgID, msg proto.Message, seq uint32, errCode uint16) *Packet {
	body := c.Encode(msg)
	return &Packet{
		MsgID:    msgID,
		Sequence: seq,
		ErrCode:  errCode,
		Body:     body,
	}
}

// Decode 解码 Body 到指定的 proto.Message
func (c *Codec) Decode(body []byte, msg proto.Message) error {
	return proto.Unmarshal(body, msg)
}

// CreateMessage 根据 MsgID 创建对应的 protobuf 消息实例
func (c *Codec) CreateMessage(msgID MsgID) proto.Message {
	switch msgID {
	case MsgLoginReq:
		return &protocol.LoginReq{}
	case MsgLoginRsp:
		return &protocol.LoginRsp{}
	case MsgRegisterReq:
		return &protocol.RegisterReq{}
	case MsgRegisterRsp:
		return &protocol.RegisterRsp{}
	case MsgCreateRoleReq:
		return &protocol.CreateRoleReq{}
	case MsgCreateRoleRsp:
		return &protocol.CreateRoleRsp{}
	case MsgLogoutReq:
		return &protocol.LogoutReq{}
	case MsgLogoutRsp:
		return &protocol.LogoutRsp{}
	case MsgHeartbeatReq:
		return &protocol.HeartbeatReq{}
	case MsgHeartbeatRsp:
		return &protocol.HeartbeatRsp{}
	case MsgReconnectReq:
		return &protocol.ReconnectReq{}
	case MsgReconnectRsp:
		return &protocol.ReconnectRsp{}
	case MsgEnterSceneReq:
		return &protocol.EnterSceneReq{}
	case MsgEnterSceneRsp:
		return &protocol.EnterSceneRsp{}
	case MsgMoveReq:
		return &protocol.MoveReq{}
	case MsgMoveBroadcast:
		return &protocol.MoveBroadcast{}
	case MsgAOIEnter:
		return &protocol.AOIEnter{}
	case MsgAOILeave:
		return &protocol.AOILeave{}
	case MsgPlayerEnterSceneNotify:
		return &protocol.PlayerEnterSceneNotify{}
	case MsgPlayerLeaveSceneNotify:
		return &protocol.PlayerLeaveSceneNotify{}
	default:
		return nil
	}
}
