package cdc

import (
	"io"
)

type simpleRepMaxContentDefinedChunker struct {
	gearTable     *GearTable
	minSizeBytes  int
	peekSizeBytes int
}

// NewSimpleRepMaxContentDefinedChunker returns a content defined
// chunker that provides the same behavior as the one returned by
// NewRepMaxContentDefinedChunker. However, this implementation is
// simpler and less efficient. It is merely provided for testing
// purposes.
func NewSimpleRepMaxContentDefinedChunker(gearTable *GearTable, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &simpleRepMaxContentDefinedChunker{
		gearTable:     gearTable,
		minSizeBytes:  minSizeBytes,
		peekSizeBytes: 2*minSizeBytes + horizonSizeBytes,
	}
}

func (c *simpleRepMaxContentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &simpleRepMaxChunkReader{
		contentDefinedChunker: c,
		peeker:                peeker,
	}
}

type simpleRepMaxChunkReader struct {
	contentDefinedChunker *simpleRepMaxContentDefinedChunker
	peeker                Peeker

	previousChunkSizeBytes int
}

func (r *simpleRepMaxChunkReader) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := r.peeker.Discard(r.previousChunkSizeBytes)
	r.previousChunkSizeBytes -= discardedSizeBytes
	if err != nil {
		return nil, err
	}

	// Gain access to the data corresponding to the next chunk(s).
	// If we're reaching the end of the input, either consume all
	// data or leave at least minSizeBytes behind. This ensures that
	// all chunks of the file are at least minSizeBytes in size,
	// assuming the file is as well.
	c := r.contentDefinedChunker
	d, err := r.peeker.Peek(c.peekSizeBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(d) < 2*c.minSizeBytes {
		if len(d) == 0 {
			return nil, io.EOF
		}
		r.previousChunkSizeBytes = len(d)
		return d, nil
	}
	d = d[:len(d)-c.minSizeBytes]

	// Compute the rolling hash leading up to the first position at
	// which we may place a cut.
	gear := &c.gearTable.values
	var initialHash uint64
	for _, b := range d[c.minSizeBytes-gearHashWindowSizeBytes : c.minSizeBytes] {
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
			r.previousChunkSizeBytes = bestChunkSizeBytes
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
