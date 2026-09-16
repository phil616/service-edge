package agent

import (
	"encoding/json"
	"log/slog"
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
	Version        int    `json:"version"`
	AdminPort      int    `json:"admin_port"`
	Fingerprint    string `json:"fingerprint,omitempty"`
	LastApplyError string `json:"last_apply_error,omitempty"`
	BinaryVersion  string `json:"binary_version,omitempty"`
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
		if err := json.Unmarshal(data, s); err != nil {
			slog.Error("invalid persisted state; reconciling full configuration", "err", err)
			s = &State{path: path}
		}
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
	oldVersion, oldBinary := s.ConfigVersion, s.FrpVersion
	s.ConfigVersion = configVersion
	if frpVersion != "" {
		s.FrpVersion = frpVersion
	}
	if err := s.persistLocked(); err != nil {
		s.ConfigVersion, s.FrpVersion = oldVersion, oldBinary
		return err
	}
	return nil
}

// ---- frpc host helpers ----

func (s *State) ConnVersion(uuid string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Connections[uuid].Version
}
func (s *State) HasConn(uuid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.Connections[uuid]
	return ok
}

// SetConn commits each successful instance independently, including partial bundles.
func (s *State) SetConn(uuid string, version, adminPort int, fingerprint string, binaryVersion ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, existed := s.Connections[uuid]
	bin := old.BinaryVersion
	if len(binaryVersion) > 0 {
		bin = binaryVersion[0]
	}
	s.Connections[uuid] = ConnState{Version: version, AdminPort: adminPort, Fingerprint: fingerprint, BinaryVersion: bin}
	if err := s.persistLocked(); err != nil {
		if existed {
			s.Connections[uuid] = old
		} else {
			delete(s.Connections, uuid)
		}
		return err
	}
	return nil
}
func (s *State) RemoveConn(uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, existed := s.Connections[uuid]
	delete(s.Connections, uuid)
	if err := s.persistLocked(); err != nil {
		if existed {
			s.Connections[uuid] = old
		}
		return err
	}
	return nil
}
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
	old := s.ConfigVersion
	s.ConfigVersion = hostVersion
	if err := s.persistLocked(); err != nil {
		s.ConfigVersion = old
		return err
	}
	return nil
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

// Failed first applies are observable too; they are not treated as applied.
func (s *State) SetConnFailure(uuid string, adminPort int, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, exists := s.Connections[uuid]
	next := old
	next.LastApplyError = message
	next.AdminPort = adminPort
	s.Connections[uuid] = next
	if err := s.persistLocked(); err != nil {
		if exists {
			s.Connections[uuid] = old
		} else {
			delete(s.Connections, uuid)
		}
		return err
	}
	return nil
}
