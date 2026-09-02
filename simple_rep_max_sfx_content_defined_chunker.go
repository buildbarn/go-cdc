package cdc

import (
	"bytes"
	"io"
)

type simpleRepMaxSfxContentDefinedChunker struct {
	nonSynchronizableContentDefinedChunker

	minSizeBytes  int
	peekSizeBytes int
}

// NewSimpleRepMaxSfxContentDefinedChunker returns a content defined
// chunker that provides the same behavior as the one returned by
// NewRepMaxSfxContentDefinedChunker. However, this implementation is
// simpler and less efficient. It is merely provided for testing
// purposes.
func NewSimpleRepMaxSfxContentDefinedChunker(minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &simpleRepMaxSfxContentDefinedChunker{
		minSizeBytes:  minSizeBytes,
		peekSizeBytes: 2*minSizeBytes + horizonSizeBytes,
	}
}

func (c *simpleRepMaxSfxContentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &simpleRepMaxSfxChunkReader{
		contentDefinedChunker: c,
		peeker:                peeker,
	}
}

func (c *simpleRepMaxSfxContentDefinedChunker) GetMaximumPeekSizeBytes() int {
	return c.peekSizeBytes
}

type simpleRepMaxSfxChunkReader struct {
	contentDefinedChunker *simpleRepMaxSfxContentDefinedChunker
	peeker                Peeker

	previousChunkSizeBytes int
}

func (r *simpleRepMaxSfxChunkReader) ReadNextChunk() ([]byte, error) {
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

	// Scan the horizon to see if there's a more suitable position
	// at which we should cut.
	for {
		bestChunkSizeBytes := c.minSizeBytes
		for i := c.minSizeBytes + 1; i <= len(d)-c.minSizeBytes; i++ {
			if bytes.Compare(d[bestChunkSizeBytes:][:c.minSizeBytes], d[i:][:c.minSizeBytes]) < 0 {
				bestChunkSizeBytes = i
			}
		}
		if bestChunkSizeBytes < 2*c.minSizeBytes {
			r.previousChunkSizeBytes = bestChunkSizeBytes
			return d[:bestChunkSizeBytes], nil
		}

		// If we were to cut at the most suitable position within
		// the horizon, we would end up with a chunk that is too
		// large. Repeat the search, limiting the size of the
		// horizon to minSizeBytes before the position that was
		// just obtained. This allows the next calls to
		// ReadNextChunk() to still consider this position again.
		d = d[:bestChunkSizeBytes]
	}
}
