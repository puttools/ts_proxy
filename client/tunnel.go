package main

import (
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"ts/proto"
)

func Connect(serverAddr string, localAddr string, tlsCfg *tls.Config, heartbeat time.Duration, logger *slog.Logger, password string) {
	backoff := time.Second
	for {
		conn, err := tls.Dial("tcp", serverAddr, tlsCfg)
		if err != nil {
			logger.Warn("connect server failed", "server", serverAddr, "err", err, "retry_in", backoff)
			time.Sleep(backoff)
			backoff = nextBackoff(backoff)
			continue
		}

		// perform auth handshake: send AUTH frame and expect ACK/FAIL
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := proto.WriteFrame(conn, &proto.Frame{Type: proto.TypeAuth, Payload: []byte(password)}); err != nil {
			logger.Warn("send auth frame failed", "err", err)
			_ = conn.Close()
			time.Sleep(backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		_ = conn.SetWriteDeadline(time.Time{})

		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		resp, err := proto.ReadFrame(conn)
		if err != nil {
			logger.Warn("read auth response failed", "err", err)
			_ = conn.Close()
			time.Sleep(backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		_ = conn.SetReadDeadline(time.Time{})

		if resp.Type == proto.TypeAuthFail {
			logger.Warn("auth failed", "msg", string(resp.Payload))
			_ = conn.Close()
			time.Sleep(backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		if resp.Type != proto.TypeAuthAck {
			// unexpected frame, but continue
			logger.Debug("unexpected auth response, continuing", "type", resp.Type)
		}

		logger.Info("connected to server", "server", serverAddr)
		backoff = time.Second

		var writeMu sync.Mutex
		pending := make(map[uint32]chan *proto.Frame)
		var pendingMu sync.Mutex

		stopHeartbeat := make(chan struct{})
		go heartbeatLoop(conn, heartbeat, stopHeartbeat, &writeMu, logger)

		err = readLoop(conn, localAddr, pending, &pendingMu, &writeMu, logger)
		close(stopHeartbeat)
		_ = conn.Close()

		pendingMu.Lock()
		for reqID, ch := range pending {
			close(ch)
			delete(pending, reqID)
		}
		pendingMu.Unlock()

		if err != nil && !errors.Is(err, io.EOF) {
			logger.Warn("disconnected from server", "err", err)
		} else {
			logger.Warn("disconnected from server")
		}
		time.Sleep(backoff)
		backoff = nextBackoff(backoff)
	}
}

func heartbeatLoop(conn net.Conn, interval time.Duration, stop <-chan struct{}, writeMu *sync.Mutex, logger *slog.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			writeMu.Lock()
			err := proto.WriteFrame(conn, &proto.Frame{Type: proto.TypeHeartbeat})
			writeMu.Unlock()
			if err != nil {
				logger.Debug("send heartbeat failed", "err", err)
				return
			}
		}
	}
}

func readLoop(
	conn net.Conn,
	localAddr string,
	pending map[uint32]chan *proto.Frame,
	pendingMu *sync.Mutex,
	writeMu *sync.Mutex,
	logger *slog.Logger,
) error {
	for {
		frame, err := proto.ReadFrame(conn)
		if err != nil {
			return err
		}

		switch frame.Type {
		case proto.TypeHeartbeatAck:
			continue
		case proto.TypeDataReq:
			pendingMu.Lock()
			ch, ok := pending[frame.ReqID]
			if !ok {
				ch = make(chan *proto.Frame, 32)
				pending[frame.ReqID] = ch
				go func(reqID uint32, initPayload []byte, reqCh chan *proto.Frame) {
					HandleRequest(conn, writeMu, reqID, initPayload, localAddr, reqCh, logger)
					pendingMu.Lock()
					delete(pending, reqID)
					pendingMu.Unlock()
				}(frame.ReqID, append([]byte(nil), frame.Payload...), ch)
				pendingMu.Unlock()
				continue
			}
			pendingMu.Unlock()
			select {
			case ch <- frame:
			default:
				logger.Debug("drop frame due to full req channel", "req_id", frame.ReqID)
			}
		case proto.TypeClose:
			pendingMu.Lock()
			ch, ok := pending[frame.ReqID]
			if ok {
				delete(pending, frame.ReqID)
				close(ch)
			}
			pendingMu.Unlock()
		case proto.TypeDataRsp:
			logger.Debug("ignore unexpected data response frame on client", "req_id", frame.ReqID)
		default:
			logger.Warn("unknown frame type", "type", frame.Type)
		}
	}
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > 60*time.Second {
		return 60 * time.Second
	}
	return next
}
