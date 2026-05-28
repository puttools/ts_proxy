package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"ts/proto"
)

func HandleRequest(
	tunnelConn net.Conn,
	writeMu *sync.Mutex,
	reqID uint32,
	initData []byte,
	localAddr string,
	inCh <-chan *proto.Frame,
	logger *slog.Logger,
) {
	sendFrame := func(f *proto.Frame) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return proto.WriteFrame(tunnelConn, f)
	}

	localConn, err := net.DialTimeout("tcp", localAddr, 10*time.Second)
	if err != nil {
		logger.Error("connect local service failed", "req_id", reqID, "err", err)
		_ = sendFrame(&proto.Frame{ReqID: reqID, Type: proto.TypeClose})
		return
	}
	defer localConn.Close()
	defer func() {
		_ = sendFrame(&proto.Frame{ReqID: reqID, Type: proto.TypeClose})
	}()

	if len(initData) > 0 {
		if err := writeAll(localConn, initData); err != nil {
			logger.Debug("write initial payload to local failed", "req_id", reqID, "err", err)
			return
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, readErr := localConn.Read(buf)
			if n > 0 {
				payload := append([]byte(nil), buf[:n]...)
				if err := sendFrame(&proto.Frame{ReqID: reqID, Type: proto.TypeDataRsp, Payload: payload}); err != nil {
					logger.Debug("send response frame failed", "req_id", reqID, "err", err)
					cancel()
					return
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					logger.Debug("local read ended", "req_id", reqID, "err", readErr)
				}
				cancel()
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-inCh:
			if !ok {
				return
			}
			if frame.Type == proto.TypeClose {
				return
			}
			if frame.Type != proto.TypeDataReq {
				continue
			}
			if err := writeAll(localConn, frame.Payload); err != nil {
				logger.Debug("write payload to local failed", "req_id", reqID, "err", err)
				cancel()
				return
			}
		}
	}
}

func writeAll(conn net.Conn, payload []byte) error {
	total := 0
	for total < len(payload) {
		n, err := conn.Write(payload[total:])
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

