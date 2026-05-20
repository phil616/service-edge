package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// State is the agent's persisted view of what's currently applied.
type State struct {
	ConfigVersion int    `json:"config_version"`
	FrpVersion    string `json:"frp_version"`

	path string
	mu   sync.Mutex
}

// LoadState reads state.json from path (returns a zero state if absent).
func LoadState(path string) *State {
	s := &State{path: path}
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, s)
	}
	return s
}

// Version returns the currently applied config version.
func (s *State) Version() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ConfigVersion
}

// Save atomically persists the state to disk.
func (s *State) Save(configVersion int, frpVersion string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ConfigVersion = configVersion
	if frpVersion != "" {
		s.FrpVersion = frpVersion
	}
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
