package proto

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	TypeHeartbeat    uint8 = 0x01
	TypeHeartbeatAck uint8 = 0x02
	TypeDataReq      uint8 = 0x03
	TypeDataRsp      uint8 = 0x04
	TypeClose        uint8 = 0x05
	// Authentication frames
	TypeAuth     uint8 = 0x06 // client -> server, payload: password bytes
	TypeAuthAck  uint8 = 0x07 // server -> client, payload empty
	TypeAuthFail uint8 = 0x08 // server -> client, payload: error message
)

const HeaderSize = 9
const MaxPayloadSize = 4 * 1024 * 1024

type Frame struct {
	ReqID   uint32
	Type    uint8
	Payload []byte
}

func WriteFrame(w io.Writer, f *Frame) error {
	if f == nil {
		return fmt.Errorf("nil frame")
	}
	if len(f.Payload) > MaxPayloadSize {
		return fmt.Errorf("payload too large: %d", len(f.Payload))
	}

	buf := make([]byte, HeaderSize+len(f.Payload))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(f.Payload)))
	binary.BigEndian.PutUint32(buf[4:8], f.ReqID)
	buf[8] = f.Type
	copy(buf[HeaderSize:], f.Payload)

	_, err := w.Write(buf)
	return err
}

func ReadFrame(r io.Reader) (*Frame, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	payloadLen := binary.BigEndian.Uint32(header[0:4])
	if payloadLen > MaxPayloadSize {
		return nil, fmt.Errorf("payload too large: %d", payloadLen)
	}

	payload := make([]byte, payloadLen)
	if payloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	return &Frame{
		ReqID:   binary.BigEndian.Uint32(header[4:8]),
		Type:    header[8],
		Payload: payload,
	}, nil
}
