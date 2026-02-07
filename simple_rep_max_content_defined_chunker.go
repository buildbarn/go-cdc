package cdc

import (
	"io"
)

type simpleRepMaxContentDefinedChunker struct {
	r             Peeker
	minSizeBytes  int
	peekSizeBytes int

	previousChunkSizeBytes int
}

// NewSimpleRepMaxContentDefinedChunker returns a content defined
// chunker that provides the same behavior as the one returned by
// NewRepMaxContentDefinedChunker. However, this implementation is
// simpler and less efficient. It is merely provided for testing
// purposes.
func NewSimpleRepMaxContentDefinedChunker(r Peeker, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &simpleRepMaxContentDefinedChunker{
		r:             r,
		minSizeBytes:  minSizeBytes,
		peekSizeBytes: 2*minSizeBytes + horizonSizeBytes,
	}
}

func (c *simpleRepMaxContentDefinedChunker) ReadNextChunk() ([]byte, error) {
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
	d, err := c.r.Peek(c.peekSizeBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(d) < 2*c.minSizeBytes {
		if len(d) == 0 {
			return nil, io.EOF
		}
		c.previousChunkSizeBytes = len(d)
		return d, nil
	}
	d = d[:len(d)-c.minSizeBytes]

	// Compute the rolling hash leading up to the first position at
	// which we may place a cut.
	var initialHash uint64
	for _, b := range d[c.minSizeBytes-64 : c.minSizeBytes] {
		initialHash = (initialHash << 1) + gear[b]
	}

	// Scan the horizon to see if there's a more suitable position
	// at which we should cut.
	for {
		hash := initialHash
		bestHash := hash
		bestCutOffsetBytes := 0
		for i, b := range d[c.minSizeBytes:] {
			hash = (hash << 1) + gear[b]
			if bestHash < hash {
				bestHash = hash
				bestCutOffsetBytes = i + 1
			}
		}
		if bestCutOffsetBytes < c.minSizeBytes {
			bestChunkSizeBytes := c.minSizeBytes + bestCutOffsetBytes
			c.previousChunkSizeBytes = bestChunkSizeBytes
			return d[:bestChunkSizeBytes], nil
		}

		// If we were to cut at the most suitable position within
		// the horizon, we would end up with a chunk that is too
		// large. Repeat the search, limiting the size of the
		// horizon to minSizeBytes before the position that was
		// just obtained. This allows the next calls to
		// ReadNextChunk() to still consider this position again.
		d = d[:bestCutOffsetBytes]
	}
}
