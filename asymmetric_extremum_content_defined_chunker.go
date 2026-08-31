package cdc

import (
	"io"
)

type asymmetricExtremumContentDefinedChunker struct {
	nonSynchronizableContentDefinedChunker

	minSizeBytes int
	maxSizeBytes int
}

// NewAsymmetricExtremumContentDefinedChunker creates a content defined
// chunker that uses the Asymmetric Extremum algorithm as described in
// the paper "AE: An Asymmetric Extremum Content Defined Chunking
// Algorithm for Fast and Bandwidth-Efficient Data Deduplication".
func NewAsymmetricExtremumContentDefinedChunker(minSizeBytes, maxSizeBytes int) ContentDefinedChunker {
	return &asymmetricExtremumContentDefinedChunker{
		minSizeBytes: minSizeBytes,
		maxSizeBytes: maxSizeBytes,
	}
}

func (c *asymmetricExtremumContentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &asymmetricExtremumChunkReader{
		contentDefinedChunker: c,
		peeker:                peeker,
	}
}

func (c *asymmetricExtremumContentDefinedChunker) GetMaximumPeekSizeBytes() int {
	return c.maxSizeBytes
}

type asymmetricExtremumChunkReader struct {
	contentDefinedChunker *asymmetricExtremumContentDefinedChunker
	peeker                Peeker

	previousChunkSizeBytes int
}

func (r *asymmetricExtremumChunkReader) ReadNextChunk() ([]byte, error) {
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

	// Process eight bytes per iteration.
	maxValue := d[0]
	maxPosition := c.minSizeBytes + 1
	i := 1
	for ; i+8 <= len(d); i += 8 {
		b := [8]byte(d[i : i+8])
		if v := b[0]; v > maxValue && maxPosition >= i+1 {
			maxValue = v
			maxPosition = i + 1 + c.minSizeBytes
		}
		if v := b[1]; v > maxValue && maxPosition >= i+2 {
			maxValue = v
			maxPosition = i + 2 + c.minSizeBytes
		}
		if v := b[2]; v > maxValue && maxPosition >= i+3 {
			maxValue = v
			maxPosition = i + 3 + c.minSizeBytes
		}
		if v := b[3]; v > maxValue && maxPosition >= i+4 {
			maxValue = v
			maxPosition = i + 4 + c.minSizeBytes
		}
		if v := b[4]; v > maxValue && maxPosition >= i+5 {
			maxValue = v
			maxPosition = i + 5 + c.minSizeBytes
		}
		if v := b[5]; v > maxValue && maxPosition >= i+6 {
			maxValue = v
			maxPosition = i + 6 + c.minSizeBytes
		}
		if v := b[6]; v > maxValue && maxPosition >= i+7 {
			maxValue = v
			maxPosition = i + 7 + c.minSizeBytes
		}
		if v := b[7]; v > maxValue && maxPosition >= i+8 {
			maxValue = v
			maxPosition = i + 8 + c.minSizeBytes
		}
		if i+8 >= maxPosition {
			r.previousChunkSizeBytes = maxPosition
			return d[:maxPosition], nil
		}
	}

	// Process the trailing bytes.
	for i < len(d) {
		if d[i] <= maxValue {
			if i+1 == maxPosition {
				r.previousChunkSizeBytes = maxPosition
				return d[:maxPosition], nil
			}
		} else {
			maxValue = d[i]
			maxPosition = i + 1 + c.minSizeBytes
		}
		i++
	}
	r.previousChunkSizeBytes = len(d)
	return d, nil
}
