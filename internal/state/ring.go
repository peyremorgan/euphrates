// Package state holds the in-memory model of euphrates: channels, groups,
// the message log and the events ring buffer.
//
// The state is the single source of truth for the UI. All mutators are
// safe for concurrent use; readers must use the provided snapshot helpers
// rather than touching internal fields.
package state

import "sync"

// Ring is a generic fixed-capacity FIFO buffer. When full, appending
// overwrites the oldest element.
type Ring[T any] struct {
	buf   []T
	start int // index of oldest element
	size  int // number of valid elements
}

// NewRing creates a Ring with the given capacity. cap must be > 0.
func NewRing[T any](capacity int) *Ring[T] {
	if capacity <= 0 {
		panic("state.NewRing: capacity must be > 0")
	}
	return &Ring[T]{buf: make([]T, capacity)}
}

// Cap returns the maximum number of elements the ring can hold.
func (r *Ring[T]) Cap() int { return len(r.buf) }

// Len returns the current number of elements.
func (r *Ring[T]) Len() int { return r.size }

// Push appends v, overwriting the oldest element when the ring is full.
func (r *Ring[T]) Push(v T) {
	if r.size < len(r.buf) {
		r.buf[(r.start+r.size)%len(r.buf)] = v
		r.size++
		return
	}
	// full: overwrite oldest, advance start
	r.buf[r.start] = v
	r.start = (r.start + 1) % len(r.buf)
}

// At returns the element at logical index i (0 = oldest).
func (r *Ring[T]) At(i int) T {
	if i < 0 || i >= r.size {
		var zero T
		return zero
	}
	return r.buf[(r.start+i)%len(r.buf)]
}

// Slice returns a new slice copy of the ring contents in oldest-first order.
func (r *Ring[T]) Slice() []T {
	out := make([]T, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(r.start+i)%len(r.buf)]
	}
	return out
}

// ForEach iterates over the ring contents in oldest-first order.
// If fn returns false the iteration stops.
func (r *Ring[T]) ForEach(fn func(v T) bool) {
	for i := 0; i < r.size; i++ {
		if !fn(r.buf[(r.start+i)%len(r.buf)]) {
			return
		}
	}
}

// Reset empties the ring without releasing the underlying storage.
func (r *Ring[T]) Reset() {
	var zero T
	for i := range r.buf {
		r.buf[i] = zero
	}
	r.start = 0
	r.size = 0
}

// guard is an embeddable RWMutex with named helpers, used by State.
type guard struct{ sync.RWMutex }
