package network

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	HeaderSize    = 12 // Length(4) + MsgID(2) + Sequence(4) + ErrCode(2)
	MaxPacketSize = 64 * 1024 // 64KB
)

// Packet 网络包结构
type Packet struct {
	MsgID    MsgID
	Sequence uint32
	ErrCode  uint16
	Body     []byte
}

// Marshal 编码为二进制: [Length(4)][MsgID(2)][Sequence(4)][ErrCode(2)][Body(N)]
func (p *Packet) Marshal() []byte {
	totalLen := HeaderSize + len(p.Body)
	buf := make([]byte, totalLen)

	binary.BigEndian.PutUint32(buf[0:4], uint32(totalLen))
	binary.BigEndian.PutUint16(buf[4:6], uint16(p.MsgID))
	binary.BigEndian.PutUint32(buf[6:10], p.Sequence)
	binary.BigEndian.PutUint16(buf[10:12], p.ErrCode)

	copy(buf[12:], p.Body)
	return buf
}

// Unmarshal 从二进制数据解码
func (p *Packet) Unmarshal(data []byte) error {
	if len(data) < HeaderSize {
		return errors.New("packet too short")
	}
	totalLen := binary.BigEndian.Uint32(data[0:4])
	if totalLen < HeaderSize || totalLen > MaxPacketSize {
		return errors.New("invalid packet length")
	}
	if int(totalLen) > len(data) {
		return errors.New("incomplete packet")
	}

	p.MsgID = MsgID(binary.BigEndian.Uint16(data[4:6]))
	p.Sequence = binary.BigEndian.Uint32(data[6:10])
	p.ErrCode = binary.BigEndian.Uint16(data[10:12])

	bodyLen := totalLen - HeaderSize
	if bodyLen > 0 {
		p.Body = make([]byte, bodyLen)
		copy(p.Body, data[12:12+bodyLen])
	} else {
		p.Body = nil
	}
	return nil
}

// PacketReader 从 Reader 中读取并组装完整包
// 处理粘包/半包，返回完整的 Packet 和剩余数据
func PacketReader(reader io.Reader, remain []byte) (*Packet, []byte, error) {
	buf := remain
	headBuf := make([]byte, HeaderSize)

	for {
		if len(buf) < HeaderSize {
			n, err := io.ReadAtLeast(reader, headBuf[:HeaderSize-len(buf)], HeaderSize-len(buf))
			if err != nil && n > 0 {
				buf = append(buf, headBuf[:n]...)
			}
			if err != nil {
				return nil, buf, err
			}
			buf = append(buf, headBuf[:n]...)
		}

		totalLen := int(binary.BigEndian.Uint32(buf[0:4]))
		if totalLen < HeaderSize || totalLen > MaxPacketSize {
			return nil, buf, errors.New("invalid packet length")
		}

		if len(buf) < totalLen {
			need := totalLen - len(buf)
			tmp := make([]byte, need)
			n, err := io.ReadAtLeast(reader, tmp, need)
			if err != nil && n > 0 {
				buf = append(buf, tmp[:n]...)
			}
			if err != nil {
				return nil, buf, err
			}
			buf = append(buf, tmp[:n]...)
		}

		// 完整包已接收
		pkt := &Packet{}
		if err := pkt.Unmarshal(buf[:totalLen]); err != nil {
			return nil, buf, err
		}
		remain = buf[totalLen:]
		return pkt, remain, nil
	}
}
