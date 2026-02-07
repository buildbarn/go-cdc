package cdc

import (
	"bufio"
)

// Peeker is used by implementations of ContentDefinedChunker to access
// data that needs to be chunked.
type Peeker interface {
	Discard(n int) (int, error)
	Peek(n int) ([]byte, error)
}

var _ Peeker = (*bufio.Reader)(nil)
