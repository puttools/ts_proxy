package proto

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestWriteReadFrameRoundTrip(t *testing.T) {
	in := &Frame{ReqID: 42, Type: TypeDataReq, Payload: []byte("hello")}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	out, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if out.ReqID != in.ReqID || out.Type != in.Type || !bytes.Equal(out.Payload, in.Payload) {
		t.Fatalf("round trip mismatch got=%+v want=%+v", out, in)
	}
}

func TestWriteReadFrameEmptyPayload(t *testing.T) {
	in := &Frame{ReqID: 1, Type: TypeHeartbeat}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, in); err != nil {
		t.Fatalf("WriteFrame() error = %v", err)
	}
	out, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame() error = %v", err)
	}
	if len(out.Payload) != 0 {
		t.Fatalf("expected empty payload, got %d", len(out.Payload))
	}
}

func TestReadFramePayloadTooLarge(t *testing.T) {
	var raw bytes.Buffer
	_ = WriteFrame(&raw, &Frame{ReqID: 9, Type: TypeDataReq, Payload: bytes.Repeat([]byte("a"), MaxPayloadSize)})
	b := raw.Bytes()
	// corrupt len to exceed MaxPayloadSize
	b[0] = 0x00
	b[1] = 0x40
	b[2] = 0x00
	b[3] = 0x01

	_, err := ReadFrame(bytes.NewReader(b))
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestReadFrameShortData(t *testing.T) {
	_, err := ReadFrame(bytes.NewReader([]byte{0x00}))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

