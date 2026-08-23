package executor

import (
	"encoding/json"
	"os"

	"github.com/Ranxy/laelia/backend/agent/atomicfile"
)

// LoadSessionState reads and JSON-decodes a session state file. A missing file
// is not an error: it returns (nil, nil) so callers cold-start.
func LoadSessionState[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state T
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// SaveSessionState JSON-encodes and atomically writes a session state file.
// It is best-effort for callers: a write failure only means the next turn
// cold-starts, never a lost message.
func SaveSessionState(path string, state any) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return atomicfile.WriteFileAtomic(path, data, 0o600)
}

// ClearSessionState drops a session state file so the next turn cold-starts.
// A missing file is not an error.
func ClearSessionState(path string) {
	_ = os.Remove(path)
}
