package utils

import (
	"bufio"
	"io"
	"sync"
)

// BufferPool manages reusable byte buffers to prevent memory leaks
type BufferPool struct {
	pool         sync.Pool
	bufSize      int
	maxPoolSize  int     // Maximum number of buffers to retain
	currentSize  int     // Current number of pooled buffers
	mu           sync.RWMutex
}

// NewBufferPool creates a new buffer pool with specified buffer size
func NewBufferPool(bufferSize int) *BufferPool {
	// Set reasonable max pool size based on buffer size
	maxPoolSize := 100 // Default max
	if bufferSize > 32*1024 {
		maxPoolSize = 50 // Fewer large buffers
	} else if bufferSize > 4*1024 {
		maxPoolSize = 75 // Medium buffers
	}
	
	return &BufferPool{
		bufSize:     bufferSize,
		maxPoolSize: maxPoolSize,
		pool: sync.Pool{
			New: func() any {
				return make([]byte, bufferSize)
			},
		},
	}
}

// NewBufferPoolWithMaxSize creates a buffer pool with custom max pool size
func NewBufferPoolWithMaxSize(bufferSize, maxPoolSize int) *BufferPool {
	return &BufferPool{
		bufSize:     bufferSize,
		maxPoolSize: maxPoolSize,
		pool: sync.Pool{
			New: func() any {
				return make([]byte, bufferSize)
			},
		},
	}
}

// Get retrieves a buffer from the pool
func (bp *BufferPool) Get() []byte {
	buf := bp.pool.Get().([]byte)
	
	// Update counter if this was from the pool (not newly created)
	bp.mu.Lock()
	if bp.currentSize > 0 {
		bp.currentSize--
	}
	bp.mu.Unlock()
	
	return buf
}

// Put returns a buffer to the pool
func (bp *BufferPool) Put(buf []byte) {
	// Only return buffers of the correct size and respect max pool size
	if len(buf) == bp.bufSize {
		bp.mu.Lock()
		defer bp.mu.Unlock()
		
		// Check if we're at capacity
		if bp.currentSize >= bp.maxPoolSize {
			// Pool is full, don't retain buffer (let it be GC'd)
			return
		}
		
		// Clear the buffer contents to prevent data leaks
		for i := range buf {
			buf[i] = 0
		}
		
		bp.currentSize++
		bp.pool.Put(buf)
	}
}

// GetStats returns current buffer pool statistics
func (bp *BufferPool) GetStats() (currentSize, maxSize, bufferSize int) {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.currentSize, bp.maxPoolSize, bp.bufSize
}

// DrainPool removes all buffers from the pool to free memory
func (bp *BufferPool) DrainPool() {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	
	// Create a new pool (old one will be GC'd)
	bp.pool = sync.Pool{
		New: func() any {
			return make([]byte, bp.bufSize)
		},
	}
	bp.currentSize = 0
}

// WriterPool manages reusable buffered writers
type WriterPool struct {
	pool sync.Pool
}

// NewWriterPool creates a new writer pool
func NewWriterPool() *WriterPool {
	return &WriterPool{
		pool: sync.Pool{
			New: func() any {
				return bufio.NewWriter(nil)
			},
		},
	}
}

// Get retrieves a writer from the pool and resets it to write to w
func (wp *WriterPool) Get(w io.Writer) *bufio.Writer {
	writer := wp.pool.Get().(*bufio.Writer)
	writer.Reset(w)
	return writer
}

// Put returns a writer to the pool after flushing
func (wp *WriterPool) Put(writer *bufio.Writer) {
	writer.Flush()
	writer.Reset(nil) // Clear the underlying writer
	wp.pool.Put(writer)
}

// ReaderPool manages reusable buffered readers
type ReaderPool struct {
	pool sync.Pool
}

// NewReaderPool creates a new reader pool
func NewReaderPool() *ReaderPool {
	return &ReaderPool{
		pool: sync.Pool{
			New: func() any {
				return bufio.NewReader(nil)
			},
		},
	}
}

// Get retrieves a reader from the pool and resets it to read from r
func (rp *ReaderPool) Get(r io.Reader) *bufio.Reader {
	reader := rp.pool.Get().(*bufio.Reader)
	reader.Reset(r)
	return reader
}

// Put returns a reader to the pool
func (rp *ReaderPool) Put(reader *bufio.Reader) {
	reader.Reset(nil) // Clear the underlying reader
	rp.pool.Put(reader)
}

// Global pools for common buffer sizes
var (
	// P2P streaming buffers (81KB) - limit to 25 buffers (~2MB max)
	P2PBufferPool = NewBufferPoolWithMaxSize(81920, 25)

	// File copy buffers (32KB) - limit to 50 buffers (~1.6MB max)
	FileBufferPool = NewBufferPoolWithMaxSize(32*1024, 50)

	// Small buffers (4KB) - limit to 100 buffers (~400KB max)
	SmallBufferPool = NewBufferPoolWithMaxSize(4*1024, 100)

	// Writer and reader pools
	GlobalWriterPool = NewWriterPool()
	GlobalReaderPool = NewReaderPool()
)

// SafeBufferOperation provides a safe way to use pooled buffers with automatic cleanup
func SafeBufferOperation(pool *BufferPool, operation func([]byte) error) error {
	buf := pool.Get()
	defer pool.Put(buf)
	return operation(buf)
}

// SafeWriterOperation provides a safe way to use pooled writers with automatic cleanup  
func SafeWriterOperation(w io.Writer, operation func(*bufio.Writer) error) error {
	writer := GlobalWriterPool.Get(w)
	defer GlobalWriterPool.Put(writer)
	return operation(writer)
}

// SafeReaderOperation provides a safe way to use pooled readers with automatic cleanup
func SafeReaderOperation(r io.Reader, operation func(*bufio.Reader) error) error {
	reader := GlobalReaderPool.Get(r)
	defer GlobalReaderPool.Put(reader)
	return operation(reader)
}
