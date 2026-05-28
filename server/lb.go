package main

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"ts/proto"
)

var ErrNoNode = errors.New("no available client")

type Node struct {
	id          string
	conn        net.Conn
	writeMu     sync.Mutex
	activeConns int32

	pending   map[uint32]chan *proto.Frame
	pendingMu sync.RWMutex
}

func NewNode(conn net.Conn) *Node {
	return &Node{
		id:      conn.RemoteAddr().String(),
		conn:    conn,
		pending: make(map[uint32]chan *proto.Frame),
	}
}

func (n *Node) ID() string {
	return n.id
}

func (n *Node) ActiveConns() int32 {
	return atomic.LoadInt32(&n.activeConns)
}

func (n *Node) Send(f *proto.Frame) error {
	n.writeMu.Lock()
	defer n.writeMu.Unlock()
	return proto.WriteFrame(n.conn, f)
}

func (n *Node) RegisterPending(reqID uint32, ch chan *proto.Frame) {
	n.pendingMu.Lock()
	n.pending[reqID] = ch
	n.pendingMu.Unlock()
}

func (n *Node) DeregisterPending(reqID uint32) (chan *proto.Frame, bool) {
	n.pendingMu.Lock()
	ch, ok := n.pending[reqID]
	delete(n.pending, reqID)
	n.pendingMu.Unlock()
	return ch, ok
}

func (n *Node) Deliver(f *proto.Frame) {
	n.pendingMu.RLock()
	ch, ok := n.pending[f.ReqID]
	if ok {
		select {
		case ch <- f:
		default:
		}
	}
	n.pendingMu.RUnlock()
}

func (n *Node) CloseAll() {
	n.pendingMu.Lock()
	for reqID, ch := range n.pending {
		close(ch)
		delete(n.pending, reqID)
	}
	n.pendingMu.Unlock()
}

type Pool struct {
	mu    sync.RWMutex
	nodes []*Node
}

func (p *Pool) Add(n *Node) {
	p.mu.Lock()
	p.nodes = append(p.nodes, n)
	p.mu.Unlock()
}

func (p *Pool) Remove(id string) {
	p.mu.Lock()
	for i, n := range p.nodes {
		if n.id == id {
			p.nodes = append(p.nodes[:i], p.nodes[i+1:]...)
			break
		}
	}
	p.mu.Unlock()
}

func (p *Pool) Pick() (*Node, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if len(p.nodes) == 0 {
		return nil, ErrNoNode
	}
	best := p.nodes[0]
	min := atomic.LoadInt32(&best.activeConns)
	for _, n := range p.nodes[1:] {
		if c := atomic.LoadInt32(&n.activeConns); c < min {
			min = c
			best = n
		}
	}
	return best, nil
}

func (p *Pool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.nodes)
}

