package cdc

import (
	"io"
)

type simpleRepLexContentDefinedChunker struct {
	nonSynchronizableContentDefinedChunker

	substitutionBox *SubstitutionBox
	minSizeBytes    int
	peekSizeBytes   int
}

func (c *simpleRepLexContentDefinedChunker) compareBytes(a, b []byte) int {
	for i := 0; i < len(a); i++ {
		if ca, cb := c.substitutionBox[a[i]], c.substitutionBox[b[i]]; ca < cb {
			return -1
		} else if ca > cb {
			return 1
		}
	}
	return 0
}

// NewSimpleRepLexContentDefinedChunker returns a content defined
// chunker that provides the same behavior as the one returned by
// NewRepLexContentDefinedChunker. However, this implementation is
// simpler and less efficient. It is merely provided for testing
// purposes.
func NewSimpleRepLexContentDefinedChunker(substitutionBox *SubstitutionBox, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &simpleRepLexContentDefinedChunker{
		substitutionBox: substitutionBox,
		minSizeBytes:    minSizeBytes,
		peekSizeBytes:   2*minSizeBytes + horizonSizeBytes,
	}
}

func (c *simpleRepLexContentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &simpleRepLexChunkReader{
		contentDefinedChunker: c,
		peeker:                peeker,
	}
}

func (c *simpleRepLexContentDefinedChunker) GetMaximumPeekSizeBytes() int {
	return c.peekSizeBytes
}

type simpleRepLexChunkReader struct {
	contentDefinedChunker *simpleRepLexContentDefinedChunker
	peeker                Peeker

	previousChunkSizeBytes int
}

func (r *simpleRepLexChunkReader) ReadNextChunk() ([]byte, error) {
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
			if c.compareBytes(d[bestChunkSizeBytes:][:c.minSizeBytes], d[i:][:c.minSizeBytes]) < 0 {
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
