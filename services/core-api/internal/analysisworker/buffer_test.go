package analysisworker

import (
	"bytes"
	"sync"
)

// lockedBuffer collects a child process's output from the goroutine the runtime
// hands it to, so a test can read the worker's own account of itself while the
// worker is still running. A worker that dies is diagnosed from this, which is the
// only place the reason is.
type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
