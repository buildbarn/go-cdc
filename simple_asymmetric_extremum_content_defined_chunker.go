package cdc

import (
	"io"
)

type simpleAsymmetricExtremumContentDefinedChunker struct {
	nonSynchronizableContentDefinedChunker

	minSizeBytes int
	maxSizeBytes int
}

// NewSimpleAsymmetricExtremumContentDefinedChunker returns a content
// defined chunker that provides the same behavior as the one returned
// by NewAsymmetricExtremumContentDefinedChunker. However, this
// implementation is simpler and less efficient. It is merely provided
// for testing purposes.
func NewSimpleAsymmetricExtremumContentDefinedChunker(minSizeBytes, maxSizeBytes int) ContentDefinedChunker {
	return &simpleAsymmetricExtremumContentDefinedChunker{
		minSizeBytes: minSizeBytes,
		maxSizeBytes: maxSizeBytes,
	}
}

func (c *simpleAsymmetricExtremumContentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &simpleAsymmetricExtremumChunkReader{
		contentDefinedChunker: c,
		peeker:                peeker,
	}
}

func (c *simpleAsymmetricExtremumContentDefinedChunker) GetMaximumPeekSizeBytes() int {
	return c.maxSizeBytes
}

type simpleAsymmetricExtremumChunkReader struct {
	contentDefinedChunker *simpleAsymmetricExtremumContentDefinedChunker
	peeker                Peeker

	previousChunkSizeBytes int
}

func (r *simpleAsymmetricExtremumChunkReader) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := r.peeker.Discard(r.previousChunkSizeBytes)
	r.previousChunkSizeBytes -= discardedSizeBytes
	if err != nil {
		return nil, err
	}

	// Gain access to the data corresponding to the next chunk(s).
	c := r.contentDefinedChunker
	d, err := r.peeker.Peek(c.maxSizeBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(d) == 0 {
		return nil, io.EOF
	}

	i := 1
	maxValue := d[i-1]
	maxPosition := i
	i++
	for i < len(d) {
		if d[i-1] <= maxValue {
			if i == maxPosition+c.minSizeBytes {
				r.previousChunkSizeBytes = i
				return d[:i], nil
			}
		} else {
			maxValue = d[i-1]
			maxPosition = i
		}
		i++
	}
	r.previousChunkSizeBytes = len(d)
	return d, nil
}
