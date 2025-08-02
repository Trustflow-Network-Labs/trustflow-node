package utils

import (
	"sync"
)

// ErrorChannelPool provides a pool of reusable error channels
// to reduce allocations for frequently created error channels
type ErrorChannelPool struct {
	pool sync.Pool
}

// NewErrorChannelPool creates a new error channel pool
func NewErrorChannelPool() *ErrorChannelPool {
	return &ErrorChannelPool{
		pool: sync.Pool{
			New: func() any {
				return make(chan error, 1)
			},
		},
	}
}

// Get retrieves an error channel from the pool
func (p *ErrorChannelPool) Get() chan error {
	return p.pool.Get().(chan error)
}

// Put returns an error channel to the pool after cleaning it
func (p *ErrorChannelPool) Put(ch chan error) {
	// Drain the channel before putting it back
	select {
	case <-ch:
		// Channel had a value, drained it
	default:
		// Channel was empty
	}

	p.pool.Put(ch)
}

// BoolChannelPool provides a pool of reusable bool channels
type BoolChannelPool struct {
	pool sync.Pool
}

// NewBoolChannelPool creates a new bool channel pool
func NewBoolChannelPool() *BoolChannelPool {
	return &BoolChannelPool{
		pool: sync.Pool{
			New: func() any {
				return make(chan bool, 1)
			},
		},
	}
}

// Get retrieves a bool channel from the pool
func (p *BoolChannelPool) Get() chan bool {
	return p.pool.Get().(chan bool)
}

// Put returns a bool channel to the pool after cleaning it
func (p *BoolChannelPool) Put(ch chan bool) {
	// Drain the channel before putting it back
	select {
	case <-ch:
		// Channel had a value, drained it
	default:
		// Channel was empty
	}

	p.pool.Put(ch)
}

// Global pool instances for common usage
var (
	GlobalErrorChannelPool = NewErrorChannelPool()
	GlobalBoolChannelPool  = NewBoolChannelPool()
)
