package cdc

import (
	"bufio"
	"io"
)

type simpleMaxContentDefinedChunker struct {
	r            *bufio.Reader
	minSizeBytes int
	maxSizeBytes int

	previousChunkSizeBytes int
}

// NewSimpleMaxContentDefinedChunker returns a content defined chunker
// that provides the same behavior as the one returned by
// NewMaxContentDefinedChunker. However, this implementation is simpler
// and less efficient. It is merely provided for testing purposes.
func NewSimpleMaxContentDefinedChunker(r io.Reader, bufferSizeBytes, minSizeBytes, maxSizeBytes int) ContentDefinedChunker {
	return &simpleMaxContentDefinedChunker{
		r:            bufio.NewReaderSize(r, bufferSizeBytes),
		minSizeBytes: minSizeBytes,
		maxSizeBytes: maxSizeBytes,
	}
}

func (c *simpleMaxContentDefinedChunker) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := c.r.Discard(c.previousChunkSizeBytes)
	c.previousChunkSizeBytes -= discardedSizeBytes
	if err != nil {
		return nil, err
	}

	// Gain access to the data corresponding to the next chunk(s).
	// If we're reaching the end of the input, either consume all
	// data or leave at least minSizeBytes behind. This ensures that
	// all chunks of the file are at least minSizeBytes in size,
	// assuming the file is as well.
	d, err := c.r.Peek(c.minSizeBytes + c.maxSizeBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(d) <= 2*c.minSizeBytes {
		if len(d) == 0 {
			return nil, io.EOF
		}
		c.previousChunkSizeBytes = len(d)
		return d, nil
	}
	d = d[:len(d)-c.minSizeBytes]

	// Compute the rolling hash leading up to the first position at
	// which we may place a cut.
	var hash uint64
	for _, b := range d[c.minSizeBytes-64 : c.minSizeBytes] {
		hash = (hash << 1) + gear[b]
	}

	// Scan the entire input to see if there's a more suitable
	// position at which we should cut.
	bestHash := hash
	bestChunkSizeBytes := c.minSizeBytes
	for i, b := range d[c.minSizeBytes:] {
		hash = (hash << 1) + gear[b]
		if bestHash < hash {
			bestHash = hash
			bestChunkSizeBytes = c.minSizeBytes + i + 1
		}
	}

	c.previousChunkSizeBytes = bestChunkSizeBytes
	return d[:bestChunkSizeBytes], nil
}
