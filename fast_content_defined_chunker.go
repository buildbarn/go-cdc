package cdc

import (
	"bufio"
	"io"

	"github.com/seehuhn/mt19937"
)

var gear [256]uint64

func init() {
	// The FastCDC paper mentions that the Gear table needs to be
	// initialized with random values. As no specific values are
	// provided, simply use the first 256 integers returned by a
	// Mersenne twister with a seed of zero.
	twister := mt19937.New()
	twister.Seed(0)
	for i := 0; i < len(gear); i++ {
		gear[i] = twister.Uint64()
	}
}

type fastContentDefinedChunker struct {
	r *bufio.Reader

	previousChunkSizeBytes int
}

// NewFastContentDefinedChunker returns a content defined chunker that
// uses the FastCDC8KB algorithm as described in the paper "The Design
// of Fast Content-Defined Chunking for Data Deduplication Based Storage
// Systems".
func NewFastContentDefinedChunker(r io.Reader, bufferSizeBytes int) ContentDefinedChunker {
	return &fastContentDefinedChunker{
		r: bufio.NewReaderSize(r, bufferSizeBytes),
	}
}

func (c *fastContentDefinedChunker) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := c.r.Discard(c.previousChunkSizeBytes)
	c.previousChunkSizeBytes -= discardedSizeBytes
	if err != nil {
		return nil, err
	}

	const (
		minSizeBytes    = 2 * 1024
		normalSizeBytes = 8 * 1024
		maxSizeBytes    = 64 * 1024
		maskS           = 0x0000d9f003530000
		maskL           = 0x0000d90003530000
	)

	// Gain access to the data corresponding to the next chunk(s).
	d, err := c.r.Peek(maxSizeBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}

	if len(d) >= normalSizeBytes {
		// Large object. Use two different bitmasks.
		var hash uint64
		for i, b := range d[minSizeBytes:normalSizeBytes] {
			hash = (hash << 1) + gear[b]
			if hash&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i
				return d[:c.previousChunkSizeBytes], nil
			}
		}
		for i, b := range d[normalSizeBytes:] {
			hash = (hash << 1) + gear[b]
			if hash&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i
				return d[:c.previousChunkSizeBytes], nil
			}
		}
	} else if len(d) >= minSizeBytes {
		// Small object. Only use a single bitmask.
		var hash uint64
		for i, b := range d[minSizeBytes:] {
			hash = (hash << 1) + gear[b]
			if hash&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i
				return d[:c.previousChunkSizeBytes], nil
			}
		}
	} else if len(d) == 0 {
		return nil, io.EOF
	}

	c.previousChunkSizeBytes = len(d)
	return d, nil
}
