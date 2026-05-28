package main

import (
	"crypto/tls"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"

	"ts/proto"
)

const serverHeartbeatTimeout = 90 * time.Second

// StartTunnel listens for incoming ts-client tunnel connections. If expectedPassword is
// non-empty, a client must send an AUTH frame with the password as the first frame.
func StartTunnel(addr string, tlsCfg *tls.Config, pool *Pool, logger *slog.Logger, expectedPassword string) error {
	ln, err := tls.Listen("tcp", addr, tlsCfg)
	if err != nil {
		return err
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Warn("accept tunnel connection failed", "err", err)
			continue
		}

		// perform authentication handshake (optional)
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		frame, err := proto.ReadFrame(conn)
		if err != nil {
			logger.Warn("read auth frame failed", "err", err)
			_ = conn.Close()
			continue
		}

		if expectedPassword != "" {
			if frame.Type != proto.TypeAuth {
				// unexpected first frame
				_ = proto.WriteFrame(conn, &proto.Frame{Type: proto.TypeAuthFail, Payload: []byte("auth required")})
				_ = conn.Close()
				continue
			}
			if string(frame.Payload) != expectedPassword {
				_ = proto.WriteFrame(conn, &proto.Frame{Type: proto.TypeAuthFail, Payload: []byte("invalid password")})
				_ = conn.Close()
				continue
			}
			// success
			_ = proto.WriteFrame(conn, &proto.Frame{Type: proto.TypeAuthAck})
		} else {
			// no password expected; if client sent auth frame, accept it; otherwise if first frame is not auth,
			// treat it as the first operational frame (put it back into a node reader). To keep things simple,
			// if first frame isn't auth we will proceed and let serveNode handle it by using a Node that
			// already has the first frame queued.
			if frame.Type == proto.TypeAuth {
				// client sent an auth but server doesn't require it; ack
				_ = proto.WriteFrame(conn, &proto.Frame{Type: proto.TypeAuthAck})
				// proceed
			} else {
				// we'll need to hand this initial frame to the node; to do that we'll create the node and
				// store the initial frame in its pending structure by spinning a goroutine that injects it
				// after node is created. For simplicity, we will attach the first frame to the connection by
				// creating a small buffered reader wrapper — but to avoid large refactors, we'll close and
				// reopen: simpler approach is to create node and start serveNode in a goroutine that first
				// delivers the initial frame via Node.Deliver after registration. We'll use a small helper.
			}
		}

		// At this point authentication passed or not required. Create node and add to pool.
		node := NewNode(conn)
		// if the first frame we read was a non-auth operational frame and server doesn't require auth,
		// deliver it to the node by spawning a goroutine that waits shortly and then delivers.
		if frame != nil && frame.Type != proto.TypeAuth {
			go func(f *proto.Frame, n *Node) {
				// small delay to ensure node has registered pending maps etc.
				time.Sleep(10 * time.Millisecond)
				n.Deliver(f)
			}(frame, node)
		}

		pool.Add(node)
		logger.Info("client connected", "client", node.ID(), "online", pool.Len())
		go serveNode(node, pool, logger)
	}
}

func serveNode(node *Node, pool *Pool, logger *slog.Logger) {
	defer func() {
		pool.Remove(node.ID())
		node.conn.Close()
		node.CloseAll()
		logger.Info("client disconnected", "client", node.ID(), "online", pool.Len())
	}()

	for {
		_ = node.conn.SetReadDeadline(time.Now().Add(serverHeartbeatTimeout))
		frame, err := proto.ReadFrame(node.conn)
		if err != nil {
			if !errors.Is(err, io.EOF) && !isNetClosed(err) {
				logger.Warn("read frame failed", "client", node.ID(), "err", err)
			}
			return
		}

		switch frame.Type {
		case proto.TypeHeartbeat:
			if err := node.Send(&proto.Frame{Type: proto.TypeHeartbeatAck}); err != nil {
				logger.Warn("send heartbeat ack failed", "client", node.ID(), "err", err)
				return
			}
		case proto.TypeDataRsp, proto.TypeClose:
			node.Deliver(frame)
		default:
			logger.Warn("unknown frame type", "client", node.ID(), "type", frame.Type)
		}
	}
}

func isNetClosed(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}
