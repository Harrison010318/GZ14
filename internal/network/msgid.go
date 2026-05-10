package network

// MsgID 消息类型标识
type MsgID uint16

const (
	// Lobby: 登录/鉴权 (0x01xx)
	MsgLoginReq      MsgID = 0x0101
	MsgLoginRsp      MsgID = 0x0102
	MsgRegisterReq   MsgID = 0x0103
	MsgRegisterRsp   MsgID = 0x0104
	MsgCreateRoleReq MsgID = 0x0105
	MsgCreateRoleRsp MsgID = 0x0106
	MsgLogoutReq     MsgID = 0x0107
	MsgLogoutRsp     MsgID = 0x0108
	MsgHeartbeatReq  MsgID = 0x0109
	MsgHeartbeatRsp  MsgID = 0x010A
	MsgReconnectReq  MsgID = 0x010B
	MsgReconnectRsp  MsgID = 0x010C

	// Scene: 场景/移动 (0x02xx)
	MsgEnterSceneReq  MsgID = 0x0201
	MsgEnterSceneRsp  MsgID = 0x0202
	MsgMoveReq        MsgID = 0x0203
	MsgMoveBroadcast  MsgID = 0x0204

	// AOI (0x03xx)
	MsgAOIEnter MsgID = 0x0301
	MsgAOILeave MsgID = 0x0302

	// 内部 RPC (0xE0xx)
	MsgPlayerEnterSceneNotify MsgID = 0xE001
	MsgPlayerLeaveSceneNotify MsgID = 0xE002
)

var msgNameMap = map[MsgID]string{
	MsgLoginReq:      "LoginReq",
	MsgLoginRsp:      "LoginRsp",
	MsgRegisterReq:   "RegisterReq",
	MsgRegisterRsp:   "RegisterRsp",
	MsgCreateRoleReq: "CreateRoleReq",
	MsgCreateRoleRsp: "CreateRoleRsp",
	MsgLogoutReq:     "LogoutReq",
	MsgLogoutRsp:     "LogoutRsp",
	MsgHeartbeatReq:  "HeartbeatReq",
	MsgHeartbeatRsp:  "HeartbeatRsp",
	MsgReconnectReq:  "ReconnectReq",
	MsgReconnectRsp:  "ReconnectRsp",
	MsgEnterSceneReq: "EnterSceneReq",
	MsgEnterSceneRsp: "EnterSceneRsp",
	MsgMoveReq:       "MoveReq",
	MsgMoveBroadcast: "MoveBroadcast",
	MsgAOIEnter:      "AOIEnter",
	MsgAOILeave:      "AOILeave",
	MsgPlayerEnterSceneNotify: "PlayerEnterSceneNotify",
	MsgPlayerLeaveSceneNotify: "PlayerLeaveSceneNotify",
}

func (m MsgID) String() string {
	if name, ok := msgNameMap[m]; ok {
		return name
	}
	return "Unknown"
}
