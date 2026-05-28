package main

import (
	"sync/atomic"
	"testing"

	"ts/proto"
)

func TestPoolPickNoNode(t *testing.T) {
	p := &Pool{}
	if _, err := p.Pick(); err != ErrNoNode {
		t.Fatalf("expected ErrNoNode, got %v", err)
	}
}

func TestPoolPickLeastActive(t *testing.T) {
	p := &Pool{}
	n1 := &Node{id: "n1", pending: map[uint32]chan *proto.Frame{}}
	n2 := &Node{id: "n2", pending: map[uint32]chan *proto.Frame{}}
	atomic.StoreInt32(&n1.activeConns, 3)
	atomic.StoreInt32(&n2.activeConns, 1)
	p.Add(n1)
	p.Add(n2)
	picked, err := p.Pick()
	if err != nil {
		t.Fatalf("Pick error = %v", err)
	}
	if picked.id != "n2" {
		t.Fatalf("expected n2, got %s", picked.id)
	}
}


