package peer

import (
	"encoding/binary"
	"fmt"
	"io"
)

type MessageID byte

const (
	MsgChoke         MessageID = 0
	MsgUnchoke       MessageID = 1
	MsgInterested    MessageID = 2
	MsgNotInterested MessageID = 3
	MsgHave          MessageID = 4
	MsgBitfield      MessageID = 5
	MsgRequest       MessageID = 6
	MsgPiece         MessageID = 7
	MsgCancel        MessageID = 8
	MsgPort          MessageID = 9
)

func (id MessageID) String() string {
	switch id {
	case MsgChoke:
		return "choke"
	case MsgUnchoke:
		return "unchoke"
	case MsgInterested:
		return "interested"
	case MsgNotInterested:
		return "not-interested"
	case MsgHave:
		return "have"
	case MsgBitfield:
		return "bitfield"
	case MsgRequest:
		return "request"
	case MsgPiece:
		return "piece"
	case MsgCancel:
		return "cancel"
	case MsgPort:
		return "port"
	default:
		return fmt.Sprintf("unknown(%d)", id)
	}
}

type Message struct {
	ID      MessageID
	Index   uint32
	Begin   uint32
	Length  uint32
	Payload []byte
}

func (m *Message) Marshal() []byte {
	switch m.ID {
	case MsgChoke, MsgUnchoke, MsgInterested, MsgNotInterested:
		buf := make([]byte, 5)
		binary.BigEndian.PutUint32(buf[0:4], 1)
		buf[4] = byte(m.ID)
		return buf

	case MsgHave:
		buf := make([]byte, 9)
		binary.BigEndian.PutUint32(buf[0:4], 5)
		buf[4] = byte(m.ID)
		binary.BigEndian.PutUint32(buf[5:9], m.Index)
		return buf

	case MsgRequest, MsgCancel:
		buf := make([]byte, 17)
		binary.BigEndian.PutUint32(buf[0:4], 13)
		buf[4] = byte(m.ID)
		binary.BigEndian.PutUint32(buf[5:9], m.Index)
		binary.BigEndian.PutUint32(buf[9:13], m.Begin)
		binary.BigEndian.PutUint32(buf[13:17], m.Length)
		return buf

	case MsgPiece:
		payloadLen := len(m.Payload)
		buf := make([]byte, 13+payloadLen)
		binary.BigEndian.PutUint32(buf[0:4], uint32(9+payloadLen))
		buf[4] = byte(m.ID)
		binary.BigEndian.PutUint32(buf[5:9], m.Index)
		binary.BigEndian.PutUint32(buf[9:13], m.Begin)
		copy(buf[13:], m.Payload)
		return buf

	case MsgBitfield:
		payloadLen := len(m.Payload)
		buf := make([]byte, 5+payloadLen)
		binary.BigEndian.PutUint32(buf[0:4], uint32(1+payloadLen))
		buf[4] = byte(m.ID)
		copy(buf[5:], m.Payload)
		return buf

	case MsgPort:
		buf := make([]byte, 7)
		binary.BigEndian.PutUint32(buf[0:4], 3)
		buf[4] = byte(m.ID)
		binary.BigEndian.PutUint16(buf[5:7], uint16(m.Index))
		return buf

	default:
		return nil
	}
}

func ReadMessage(r io.Reader) (*Message, error) {
	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf)

	if length == 0 {
		return nil, nil
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read message body: %w", err)
	}

	msg := &Message{ID: MessageID(buf[0])}

	switch msg.ID {
	case MsgChoke, MsgUnchoke, MsgInterested, MsgNotInterested:
	case MsgHave:
		msg.Index = binary.BigEndian.Uint32(buf[1:5])
	case MsgBitfield:
		msg.Payload = make([]byte, length-1)
		copy(msg.Payload, buf[1:])
	case MsgRequest, MsgCancel:
		msg.Index = binary.BigEndian.Uint32(buf[1:5])
		msg.Begin = binary.BigEndian.Uint32(buf[5:9])
		msg.Length = binary.BigEndian.Uint32(buf[9:13])
	case MsgPiece:
		msg.Index = binary.BigEndian.Uint32(buf[1:5])
		msg.Begin = binary.BigEndian.Uint32(buf[5:9])
		msg.Payload = make([]byte, length-9)
		copy(msg.Payload, buf[9:])
	case MsgPort:
		msg.Index = uint32(binary.BigEndian.Uint16(buf[1:3]))
	}

	return msg, nil
}

func SendMessage(w io.Writer, msg *Message) error {
	data := msg.Marshal()
	if data == nil {
		return fmt.Errorf("send: unknown message type %d", msg.ID)
	}
	_, err := w.Write(data)
	return err
}
