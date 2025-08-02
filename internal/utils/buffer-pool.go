package utils

import (
	"bufio"
	"io"
	"sync"
)

// BufferPool manages reusable byte buffers to prevent memory leaks
type BufferPool struct {
	pool    sync.Pool
	bufSize int
}

// NewBufferPool creates a new buffer pool with specified buffer size
func NewBufferPool(bufferSize int) *BufferPool {
	return &BufferPool{
		bufSize: bufferSize,
		pool: sync.Pool{
			New: func() any {
				return make([]byte, bufferSize)
			},
		},
	}
}

// Get retrieves a buffer from the pool
func (bp *BufferPool) Get() []byte {
	return bp.pool.Get().([]byte)
}

// Put returns a buffer to the pool
func (bp *BufferPool) Put(buf *[]byte) {
	// Only return buffers of the correct size
	if len(*buf) == bp.bufSize {
		bp.pool.Put(buf)
	}
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
	// P2P streaming buffers (81KB default)
	P2PBufferPool = NewBufferPool(81920)

	// File copy buffers (32KB)
	FileBufferPool = NewBufferPool(32 * 1024)

	// Small buffers (4KB) for general use
	SmallBufferPool = NewBufferPool(4 * 1024)

	// Writer and reader pools
	GlobalWriterPool = NewWriterPool()
	GlobalReaderPool = NewReaderPool()
)
