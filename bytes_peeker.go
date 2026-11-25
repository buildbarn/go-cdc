package cdc

import (
	"bufio"
	"io"
)

type bytesPeeker struct {
	data []byte
}

// NewBytesPeeker creates a Peeker that provides access to data that is
// already stored in memory. Unlike bufio.Reader, it does not copy the
// data into a separate buffer, and it permits peeking arbitrarily far
// ahead.
//
// Slices returned by Peek() refer to the data provided, which must
// therefore not be modified while the Peeker is in use.
func NewBytesPeeker(data []byte) Peeker {
	return &bytesPeeker{data: data}
}

func (p *bytesPeeker) Discard(n int) (int, error) {
	if n < 0 {
		return 0, bufio.ErrNegativeCount
	}
	if n > len(p.data) {
		n = len(p.data)
		p.data = p.data[n:]
		return n, io.EOF
	}
	p.data = p.data[n:]
	return n, nil
}

func (p *bytesPeeker) Peek(n int) ([]byte, error) {
	if n < 0 {
		return nil, bufio.ErrNegativeCount
	}
	if n > len(p.data) {
		return p.data[:len(p.data):len(p.data)], io.EOF
	}
	return p.data[:n:n], nil
}
