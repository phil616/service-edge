package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// State is the agent's persisted view of what's currently applied. For frps it
// tracks the single applied config version; for an frpc host it also tracks the
// applied version of each managed connection (so the reconciler can decide which
// frpc processes to (re)apply or stop).
// ConnState is the applied state of one frpc connection on a host.
type ConnState struct {
	Version   int `json:"version"`
	AdminPort int `json:"admin_port"`
}

type State struct {
	ConfigVersion int                  `json:"config_version"` // frps: applied; frpc: host aggregate
	FrpVersion    string               `json:"frp_version"`
	Connections   map[string]ConnState `json:"connections,omitempty"` // connUUID -> state

	path string
	mu   sync.Mutex
}

// LoadState reads state.json from path (returns a zero state if absent).
func LoadState(path string) *State {
	s := &State{path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, s)
	}
	if s.Connections == nil {
		s.Connections = map[string]ConnState{}
	}
	return s
}

// Version returns the currently applied config version (frps) or host aggregate.
func (s *State) Version() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ConfigVersion
}

// Save atomically persists the single (frps) state.
func (s *State) Save(configVersion int, frpVersion string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ConfigVersion = configVersion
	if frpVersion != "" {
		s.FrpVersion = frpVersion
	}
	return s.persistLocked()
}

// ---- frpc host helpers ----

func (s *State) ConnVersion(uuid string) int { s.mu.Lock(); defer s.mu.Unlock(); return s.Connections[uuid].Version }
func (s *State) HasConn(uuid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.Connections[uuid]
	return ok
}
func (s *State) SetConn(uuid string, version, adminPort int) {
	s.mu.Lock()
	s.Connections[uuid] = ConnState{Version: version, AdminPort: adminPort}
	s.mu.Unlock()
}
func (s *State) RemoveConn(uuid string) { s.mu.Lock(); delete(s.Connections, uuid); s.mu.Unlock() }
func (s *State) ConnUUIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.Connections))
	for u := range s.Connections {
		out = append(out, u)
	}
	return out
}

// ConnEntries returns a copy of the managed connections (uuid -> state).
func (s *State) ConnEntries() map[string]ConnState {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]ConnState, len(s.Connections))
	for u, st := range s.Connections {
		out[u] = st
	}
	return out
}

// SaveHost sets the host aggregate version and persists.
func (s *State) SaveHost(hostVersion int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ConfigVersion = hostVersion
	return s.persistLocked()
}

func (s *State) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
