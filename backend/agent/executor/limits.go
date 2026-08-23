package executor

import "time"

const (
	// DefaultMaxTimeoutSeconds is the fallback per-turn timeout shared by the
	// ACP, thread, and pi executors.
	DefaultMaxTimeoutSeconds = 1800
	// DefaultMaxEventCount is the fallback structured-event cap shared by the
	// ACP, thread, and pi executors.
	DefaultMaxEventCount = 10000
	// DefaultMaxOutputBytes is the fallback total text-output cap shared by
	// the ACP, thread, and pi executors.
	DefaultMaxOutputBytes = 1 << 20
	// DefaultOutputFlushBytes is the fallback buffered-text flush threshold
	// shared by the ACP, thread, and pi executors.
	DefaultOutputFlushBytes = 4096
)

// Limits carries the runtime resource limits shared by the ACP, thread, and pi
// executors. Each executor config embeds it so the same fields and defaults
// are defined once instead of being duplicated per runtime.
type Limits struct {
	MaxTimeoutSeconds int32 `yaml:"max_timeout_seconds"`
	MaxEventCount     int32 `yaml:"max_event_count"`
	MaxOutputBytes    int64 `yaml:"max_output_bytes"`
	OutputFlushBytes  int32 `yaml:"output_flush_bytes"`

	// StartupTimeout bounds the subprocess startup handshake (spawn + first
	// protocol round trip). A subprocess that does not complete it within this
	// window is failed fast at ~StartupTimeout instead of hanging to
	// MaxTimeoutSeconds. Each runtime keeps its own default when zero.
	StartupTimeout time.Duration `yaml:"startup_timeout"`
}
