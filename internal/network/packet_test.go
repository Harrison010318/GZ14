package network

import (
	"bytes"
	"testing"
)

func TestPacketMarshalUnmarshal(t *testing.T) {
	pkt := &Packet{
		MsgID:    MsgLoginReq,
		Sequence: 42,
		ErrCode:  0,
		Body:     []byte("hello"),
	}

	data := pkt.Marshal()

	got := &Packet{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if got.MsgID != MsgLoginReq {
		t.Errorf("MsgID = %v, want %v", got.MsgID, MsgLoginReq)
	}
	if got.Sequence != 42 {
		t.Errorf("Sequence = %d, want 42", got.Sequence)
	}
	if got.ErrCode != 0 {
		t.Errorf("ErrCode = %d, want 0", got.ErrCode)
	}
	if string(got.Body) != "hello" {
		t.Errorf("Body = %q, want %q", string(got.Body), "hello")
	}
}

func TestPacketEmptyBody(t *testing.T) {
	pkt := &Packet{
		MsgID:    MsgHeartbeatReq,
		Sequence: 1,
		ErrCode:  0,
		Body:     nil,
	}

	data := pkt.Marshal()
	got := &Packet{}
	if err := got.Unmarshal(data); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if got.MsgID != MsgHeartbeatReq {
		t.Errorf("MsgID mismatch")
	}
	if got.Body != nil {
		t.Errorf("expected nil body, got %v", got.Body)
	}
}

func TestPacketInvalidLength(t *testing.T) {
	// 长度字段小于 HeaderSize
	data := make([]byte, 4)
	data[0] = 0
	data[1] = 0
	data[2] = 0
	data[3] = 5 // totalLen = 5 < HeaderSize(12)

	pkt := &Packet{}
	if err := pkt.Unmarshal(data); err == nil {
		t.Error("expected error for invalid length")
	}
}

func TestPacketReaderSingle(t *testing.T) {
	pkt := &Packet{
		MsgID:    MsgLoginReq,
		Sequence: 1,
		Body:     []byte("test body"),
	}
	data := pkt.Marshal()

	reader := bytes.NewReader(data)
	got, remain, err := PacketReader(reader, nil)
	if err != nil {
		t.Fatalf("PacketReader failed: %v", err)
	}
	if got.MsgID != MsgLoginReq {
		t.Errorf("MsgID mismatch")
	}
	if string(got.Body) != "test body" {
		t.Errorf("Body mismatch: got %q", string(got.Body))
	}
	if len(remain) != 0 {
		t.Errorf("expected no remaining data, got %d bytes", len(remain))
	}
}

func TestPacketReaderSticky(t *testing.T) {
	// 模拟粘包：两个包连在一起发送
	pkt1 := &Packet{MsgID: MsgLoginReq, Sequence: 1, Body: []byte("packet1")}
	pkt2 := &Packet{MsgID: MsgMoveReq, Sequence: 2, Body: []byte("packet2")}

	data1 := pkt1.Marshal()
	data2 := pkt2.Marshal()
	combined := append(data1, data2...)

	reader := bytes.NewReader(combined)

	// 读第一个包
	got1, remain, err := PacketReader(reader, nil)
	if err != nil {
		t.Fatalf("first packet read failed: %v", err)
	}
	if string(got1.Body) != "packet1" {
		t.Errorf("first packet body: got %q", string(got1.Body))
	}

	// 读第二个包（从剩余数据中读取）
	got2, remain, err := PacketReader(reader, remain)
	if err != nil {
		t.Fatalf("second packet read failed: %v", err)
	}
	if string(got2.Body) != "packet2" {
		t.Errorf("second packet body: got %q", string(got2.Body))
	}
	if len(remain) != 0 {
		t.Errorf("expected no remaining data")
	}
}

func TestPacketReaderHalfPacket(t *testing.T) {
	// 模拟半包：只发送部分数据
	pkt := &Packet{MsgID: MsgLoginReq, Sequence: 1, Body: []byte("half packet test")}
	fullData := pkt.Marshal()

	// 只发送前 10 个字节
	partialData := fullData[:10]
	reader := bytes.NewReader(partialData)

	// 第一轮读应该返回 ErrUnexpectedEOF
	_, remain, err := PacketReader(reader, nil)
	if err == nil {
		t.Fatal("expected error for half packet")
	}

	// 补充剩余数据
	restData := fullData[10:]
	reader2 := bytes.NewReader(restData)

	// 第二轮读应该拿到完整包（加上之前剩余的 partial header）
	got, remain, err := PacketReader(reader2, remain)
	if err != nil {
		t.Fatalf("complete packet read failed: %v", err)
	}
	if string(got.Body) != "half packet test" {
		t.Errorf("body mismatch: got %q", string(got.Body))
	}
	if len(remain) != 0 {
		t.Errorf("expected no remaining data")
	}
}

func TestMsgIDString(t *testing.T) {
	tests := []struct {
		id   MsgID
		name string
	}{
		{MsgLoginReq, "LoginReq"},
		{MsgLoginRsp, "LoginRsp"},
		{MsgMoveReq, "MoveReq"},
		{MsgMoveBroadcast, "MoveBroadcast"},
		{MsgAOIEnter, "AOIEnter"},
		{MsgAOILeave, "AOILeave"},
		{MsgID(0xFFFF), "Unknown"},
	}

	for _, tt := range tests {
		if got := tt.id.String(); got != tt.name {
			t.Errorf("MsgID(0x%X).String() = %q, want %q", uint16(tt.id), got, tt.name)
		}
	}
}
