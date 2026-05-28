package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync/atomic"
	"time"

	"ts/proto"
)

func StartProxy(addr string, pool *Pool, timeout time.Duration, reqIDGen *uint32, logger *slog.Logger) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Warn("accept proxy connection failed", "err", err)
			continue
		}
		go handleConn(conn, pool, timeout, reqIDGen, logger)
	}
}

func handleConn(nginxConn net.Conn, pool *Pool, timeout time.Duration, reqIDGen *uint32, logger *slog.Logger) {
	defer nginxConn.Close()

	node, err := pool.Pick()
	if err != nil {
		logger.Warn("no available client")
		return
	}

	reqID := atomic.AddUint32(reqIDGen, 1)
	atomic.AddInt32(&node.activeConns, 1)
	defer atomic.AddInt32(&node.activeConns, -1)

	respCh := make(chan *proto.Frame, 32)
	node.RegisterPending(reqID, respCh)
	defer node.DeregisterPending(reqID)
	defer func() {
		_ = node.Send(&proto.Frame{ReqID: reqID, Type: proto.TypeClose})
	}()

	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		buf := make([]byte, 32*1024)
		for {
			_ = nginxConn.SetReadDeadline(time.Now().Add(timeout))
			n, readErr := nginxConn.Read(buf)
			if n > 0 {
				payload := append([]byte(nil), buf[:n]...)
				if sendErr := node.Send(&proto.Frame{ReqID: reqID, Type: proto.TypeDataReq, Payload: payload}); sendErr != nil {
					logger.Warn("send request frame failed", "req_id", reqID, "client", node.ID(), "err", sendErr)
					cancel()
					return
				}
			}
			if readErr != nil {
				if !errors.Is(readErr, io.EOF) {
					logger.Debug("nginx read ended", "req_id", reqID, "err", readErr)
				}
				cancel()
				return
			}
		}
	}()

	var result = "ok"
	for {
		select {
		case <-ctx.Done():
			logger.Info("request finished", "req_id", reqID, "client", node.ID(), "cost", time.Since(start), "result", result)
			return
		case frame, ok := <-respCh:
			if !ok {
				result = "client_disconnected"
				logger.Info("request finished", "req_id", reqID, "client", node.ID(), "cost", time.Since(start), "result", result)
				return
			}
			if frame.Type == proto.TypeClose {
				result = "closed"
				logger.Info("request finished", "req_id", reqID, "client", node.ID(), "cost", time.Since(start), "result", result)
				return
			}
			if frame.Type != proto.TypeDataRsp {
				continue
			}
			_ = nginxConn.SetWriteDeadline(time.Now().Add(timeout))
			if _, err := nginxConn.Write(frame.Payload); err != nil {
				result = "write_error"
				logger.Debug("write back to nginx failed", "req_id", reqID, "err", err)
				cancel()
			}
		}
	}
}

