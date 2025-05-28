package utils

type FIFOBuffer struct {
	elements []any
	size     int
	start    int
	count    int
}

// Initialize the buffer with a fixed size
func NewFIFOBuffer(size int) *FIFOBuffer {
	return &FIFOBuffer{
		elements: make([]any, size),
		size:     size,
	}
}

// Add a new entry (overwrites oldest if full)
func (rb *FIFOBuffer) Add(element any) {
	rb.elements[(rb.start+rb.count)%rb.size] = element

	if rb.count < rb.size {
		rb.count++
	} else {
		rb.start = (rb.start + 1) % rb.size
	}
}

// Remove deletes an element by value (first occurrence)
func (rb *FIFOBuffer) Remove(target any) bool {
	if rb.count == 0 {
		return false
	}

	index := -1
	for i := 0; i < rb.count; i++ {
		pos := (rb.start + i) % rb.size
		if rb.elements[pos] == target {
			index = pos
			break
		}
	}

	if index == -1 {
		return false // Not found
	}

	// Shift elements to fill the gap
	for i := index; i != (rb.start+rb.count-1)%rb.size; i = (i + 1) % rb.size {
		next := (i + 1) % rb.size
		rb.elements[i] = rb.elements[next]
	}

	rb.count--
	return true
}

// Get all entries in order
func (rb *FIFOBuffer) Entries() []any {
	entries := make([]any, rb.count)
	for i := range rb.count {
		entries[i] = rb.elements[(rb.start+i)%rb.size]
	}
	return entries
}
