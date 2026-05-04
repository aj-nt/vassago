// This file is part of Mnemosyne.
// See LICENSE-AGPLv3 for license information.

package tools

import (
	"sync"
)

// RingBuffer is a thread-safe ring buffer that overwrites old data when full.
// It tracks total bytes written so callers can request only new data via ReadFrom.
type RingBuffer struct {
	buf     []byte
	start   int // index of oldest byte
	count   int // number of valid bytes
	totalW  int // total bytes ever written
	mu      sync.Mutex
}

// NewRingBuffer creates a new ring buffer with the given capacity in bytes.
func NewRingBuffer(capacity int) *RingBuffer {
	return &RingBuffer{
		buf: make([]byte, capacity),
	}
}

// Write appends data to the ring buffer, overwriting the oldest data if full.
// Implements io.Writer.
func (rb *RingBuffer) Write(data []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, b := range data {
		if rb.count == len(rb.buf) {
			// Buffer full, overwrite oldest
			rb.buf[rb.start] = b
			rb.start = (rb.start + 1) % len(rb.buf)
		} else {
			pos := (rb.start + rb.count) % len(rb.buf)
			rb.buf[pos] = b
			rb.count++
		}
		rb.totalW++
	}
	return len(data), nil
}

// ReadFrom returns all data written since totalOffset, truncated to buffer capacity.
// Pass totalOffset=0 to get all available data.
func (rb *RingBuffer) ReadFrom(totalOffset int) []byte {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if totalOffset >= rb.totalW {
		return nil
	}

	// How many bytes of new data are available
	available := rb.totalW - totalOffset
	// Can only return what's in the buffer
	if available > rb.count {
		available = rb.count
	}

	result := make([]byte, available)
	// Read from (start + count - available) position
	readStart := (rb.start + rb.count - available) % len(rb.buf)
	for i := 0; i < available; i++ {
		result[i] = rb.buf[(readStart+i)%len(rb.buf)]
	}
	return result
}

// TotalWritten returns the total number of bytes ever written.
func (rb *RingBuffer) TotalWritten() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.totalW
}

// Len returns the number of bytes currently in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}