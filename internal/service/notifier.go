package service

import "sync"

// Notifier lets config-changing operations wake agents that are parked in a
// long-poll for a specific UUID. Each waiter registers a buffered channel; a
// publish closes/signals all current waiters for that UUID.
type Notifier struct {
	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
}

func NewNotifier() *Notifier {
	return &Notifier{waiters: make(map[string]map[chan struct{}]struct{})}
}

// Subscribe registers a waiter for uuid and returns its channel plus an
// unsubscribe func that must be called when the wait ends.
func (n *Notifier) Subscribe(uuid string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	set, ok := n.waiters[uuid]
	if !ok {
		set = make(map[chan struct{}]struct{})
		n.waiters[uuid] = set
	}
	set[ch] = struct{}{}
	n.mu.Unlock()

	return ch, func() {
		n.mu.Lock()
		if s, ok := n.waiters[uuid]; ok {
			delete(s, ch)
			if len(s) == 0 {
				delete(n.waiters, uuid)
			}
		}
		n.mu.Unlock()
	}
}

// Publish wakes every current waiter for uuid.
func (n *Notifier) Publish(uuid string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.waiters[uuid] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
