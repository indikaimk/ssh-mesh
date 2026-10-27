package broker

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
)

var (
	ErrAgentNotFound = errors.New("agent not found or offline")
	ErrSessionClosed = errors.New("underlying multiplexed session closed")
)

// Broker defines the abstraction for managing agent sessions and routing streams.
// This interface allows swapping in-memory single-node routing for a gRPC mesh or Redis Pub/Sub later.
type Broker interface {
	Register(info *protocol.AgentInfo, session *yamux.Session) error
	Unregister(agentID string)
	Get(agentID string) (*protocol.AgentInfo, error)
	List() []*protocol.AgentInfo
	DialStream(ctx context.Context, agentID string) (net.Conn, error)
	UpdateHeartbeat(agentID string)
}

type agentEntry struct {
	info    *protocol.AgentInfo
	session *yamux.Session
}

// MemoryBroker is an in-memory implementation of the Broker interface.
type MemoryBroker struct {
	mu     sync.RWMutex
	agents map[string]*agentEntry
}

// NewMemoryBroker creates a new in-memory agent session broker.
func NewMemoryBroker() *MemoryBroker {
	return &MemoryBroker{
		agents: make(map[string]*agentEntry),
	}
}

func (b *MemoryBroker) Register(info *protocol.AgentInfo, session *yamux.Session) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	info.ConnectedAt = time.Now()
	info.LastSeenAt = time.Now()
	info.Status = "online"

	// If there was an old session, close it cleanly
	if old, exists := b.agents[info.ID]; exists && old.session != nil {
		_ = old.session.Close()
	}

	b.agents[info.ID] = &agentEntry{
		info:    info,
		session: session,
	}
	return nil
}

func (b *MemoryBroker) Unregister(agentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if entry, exists := b.agents[agentID]; exists {
		entry.info.Status = "offline"
		if entry.session != nil {
			_ = entry.session.Close()
			entry.session = nil
		}
	}
}

func (b *MemoryBroker) Get(agentID string) (*protocol.AgentInfo, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	entry, exists := b.agents[agentID]
	if !exists {
		return nil, ErrAgentNotFound
	}
	// Return a copy to avoid race conditions
	infoCopy := *entry.info
	return &infoCopy, nil
}

func (b *MemoryBroker) List() []*protocol.AgentInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()

	list := make([]*protocol.AgentInfo, 0, len(b.agents))
	for _, entry := range b.agents {
		infoCopy := *entry.info
		list = append(list, &infoCopy)
	}
	return list
}

func (b *MemoryBroker) UpdateHeartbeat(agentID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if entry, exists := b.agents[agentID]; exists {
		entry.info.LastSeenAt = time.Now()
	}
}

func (b *MemoryBroker) DialStream(ctx context.Context, agentID string) (net.Conn, error) {
	b.mu.RLock()
	entry, exists := b.agents[agentID]
	b.mu.RUnlock()

	if !exists || entry.session == nil || entry.session.IsClosed() {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	// Open a new multiplexed channel to the agent's embedded SSH listener
	stream, err := entry.session.Open()
	if err != nil {
		return nil, fmt.Errorf("failed opening stream to agent %s: %w", agentID, err)
	}

	return stream, nil
}
