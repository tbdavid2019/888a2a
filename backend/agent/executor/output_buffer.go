package executor

import (
	"strings"
	"sync"
	"time"

	v1pb "github.com/Ranxy/laelia/backend/generated-go/v1"
)

const (
	// OutputBufferSize bounds the in-flight channel between a Runtime and the
	// stream pump that forwards chunks to the manager.
	OutputBufferSize = 1024

	// FlushOutputInterval is the periodic buffer-flush cadence shared by the
	// ACP, thread, and pi executors so their live-stream latency matches.
	FlushOutputInterval = 500 * time.Millisecond
)

// OutputBuffer batches STDOUT/SYSTEM/ASSISTANT text deltas into consolidated
// CommandOutput chunks. Streaming runtimes emit per-token text deltas; without
// batching each token becomes its own command_output row. LLM tokens carry
// their own whitespace, so concatenating deltas before flushing reproduces the
// original text exactly. Flushed on the byte threshold, a periodic tick,
// tool-call boundaries, and at finish.
type OutputBuffer struct {
	mu        sync.Mutex
	stdout    strings.Builder
	system    strings.Builder
	assistant strings.Builder
	order     []v1pb.CommandOutput_StreamType
}

// Append buffers text for the given stream, recording first-seen stream order
// so Flush emits streams in the order they first appeared.
func (b *OutputBuffer) Append(streamType v1pb.CommandOutput_StreamType, text string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch streamType {
	case v1pb.CommandOutput_STDOUT:
		if b.stdout.Len() == 0 {
			b.order = append(b.order, streamType)
		}
		_, _ = b.stdout.WriteString(text)
	case v1pb.CommandOutput_SYSTEM:
		if b.system.Len() == 0 {
			b.order = append(b.order, streamType)
		}
		_, _ = b.system.WriteString(text)
	case v1pb.CommandOutput_ASSISTANT:
		if b.assistant.Len() == 0 {
			b.order = append(b.order, streamType)
		}
		_, _ = b.assistant.WriteString(text)
	default:
		// Other stream types (STDERR) are sent directly, not buffered.
	}
}

// TotalLen returns the number of buffered bytes across all streams.
func (b *OutputBuffer) TotalLen() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stdout.Len() + b.system.Len() + b.assistant.Len()
}

// HasContent reports whether any buffered text is pending.
func (b *OutputBuffer) HasContent() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stdout.Len() > 0 || b.system.Len() > 0 || b.assistant.Len() > 0
}

// Flush drains the buffer through send, preserving first-seen stream order.
// send is the executor's sendOutput method value; passing it as a callback
// keeps OutputBuffer independent of the concrete executor type.
func (b *OutputBuffer) Flush(send func(streamType v1pb.CommandOutput_StreamType, content string)) {
	b.mu.Lock()
	stdout := b.stdout.String()
	b.stdout.Reset()
	system := b.system.String()
	b.system.Reset()
	assistant := b.assistant.String()
	b.assistant.Reset()
	order := b.order
	b.order = b.order[:0]
	b.mu.Unlock()

	for _, st := range order {
		switch st {
		case v1pb.CommandOutput_STDOUT:
			if stdout != "" {
				send(v1pb.CommandOutput_STDOUT, stdout)
				stdout = ""
			}
		case v1pb.CommandOutput_SYSTEM:
			if system != "" {
				send(v1pb.CommandOutput_SYSTEM, system)
				system = ""
			}
		case v1pb.CommandOutput_ASSISTANT:
			if assistant != "" {
				send(v1pb.CommandOutput_ASSISTANT, assistant)
				assistant = ""
			}
		default:
		}
	}
}
