package controller

import (
	"sync"
	"time"
)

// NodeVerificationState tracks the verification state of a node
type NodeVerificationState string

const (
	StateUnverified NodeVerificationState = "unverified"
	StatePending    NodeVerificationState = "pending"
	StateInProgress NodeVerificationState = "in_progress"
	StateVerified   NodeVerificationState = "verified"
	StateFailed     NodeVerificationState = "failed"
	StateDeleting   NodeVerificationState = "deleting"
)

// NodeState represents the current state of a node's verification process
type NodeState struct {
	NodeName      string
	State         NodeVerificationState
	WorkerJobName string // Changed from WorkerPodName
	JobUID        string // Added for better job tracking
	LastAttempt   time.Time
	AttemptCount  int
	LastError     string
	CreatedAt     time.Time
	VerifiedAt    *time.Time
}

// StateCache maintains in-memory state of node verifications
type StateCache struct {
	mu     sync.RWMutex
	states map[string]*NodeState
}

// NewStateCache creates a new state cache
func NewStateCache() *StateCache {
	return &StateCache{
		states: make(map[string]*NodeState),
	}
}

// Get retrieves the state for a given node
func (c *StateCache) Get(nodeName string) *NodeState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.states[nodeName]
}

// Set stores or updates the state for a given node
func (c *StateCache) Set(nodeName string, state *NodeState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.states[nodeName] = state
}

// Delete removes the state for a given node
func (c *StateCache) Delete(nodeName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, nodeName)
}

// List returns all node states
func (c *StateCache) List() []*NodeState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	states := make([]*NodeState, 0, len(c.states))
	for _, state := range c.states {
		states = append(states, state)
	}
	return states
}

// CountByState returns the number of nodes in each state
func (c *StateCache) CountByState() map[NodeVerificationState]int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	counts := make(map[NodeVerificationState]int)
	for _, state := range c.states {
		counts[state.State]++
	}
	return counts
}

// GetAll returns a copy of all node states (for metrics)
func (c *StateCache) GetAll() map[string]*NodeState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*NodeState, len(c.states))
	for k, v := range c.states {
		result[k] = v
	}
	return result
}
