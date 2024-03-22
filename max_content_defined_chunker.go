package cdc

import (
	"bufio"
	"io"
)

type chunk struct {
	hash uint64
	end  int
}

type maxContentDefinedChunker struct {
	r            *bufio.Reader
	minSizeBytes int
	maxSizeBytes int

	chunks []chunk
}

// NewMaxContentDefinedChunker returns a content defined chunker that
// uses an algorithm that is inspired by FastCDC. Instead of placing
// cutting points at the first position at which a rolling hash has a
// given number of zero bits, it uses the position at which the rolling
// hash is maximized.
//
// This approach requires the algorithm to compute the rolling hash up
// to maxSizeBytes-minSizeBytes past the eventually chosen cutting
// point. To prevent this from being wasteful, this implementation
// stores cutting points on a stack that is preserved across calls.
//
// Throughput of this implementation is supposed to be nearly identical
// to plain FastCDC. Due to the sizes of chunks being uniformly
// distributed as opposed to normal-like, the spread in chunk size is
// smaller. Furthermore, it is expected that this distribution also
// causes the sequence of chunks to converge more quickly after parts
// that differ between files have finished processing.
func NewMaxContentDefinedChunker(r io.Reader, bufferSizeBytes, minSizeBytes, maxSizeBytes int) ContentDefinedChunker {
	return &maxContentDefinedChunker{
		r:            bufio.NewReaderSize(r, bufferSizeBytes),
		minSizeBytes: minSizeBytes,
		maxSizeBytes: maxSizeBytes,
		chunks:       make([]chunk, 1, maxSizeBytes/minSizeBytes+2),
	}
}

func (c *maxContentDefinedChunker) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := c.r.Discard(c.chunks[0].end)
	for i := range c.chunks {
		c.chunks[i].end -= discardedSizeBytes
	}
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
		c.chunks = append(c.chunks[:0], chunk{end: len(d)})
		return d, nil
	}
	d = d[:len(d)-c.minSizeBytes]

	// Extract the two final chunks from the stack. The last chunk
	// denotes where the previous call stopped hashing the input.
	// The second from last chunk is used to derive the size of the
	// last chunk and to determine whether a new potential cutting
	// point is found.
	var previousChunk, currentChunk chunk
	var oldChunks []chunk
	if len(c.chunks) > 2 {
		previousChunk, currentChunk = c.chunks[len(c.chunks)-2], c.chunks[len(c.chunks)-1]
		oldChunks = append(c.chunks[:0], c.chunks[1:len(c.chunks)-2]...)
	} else {
		// This is the very first chunk, or the previous chunk
		// was larger than maxSizeBytes-minSizeBytes. We know
		// that the first minSizeBytes positions can't contain a
		// cut. Skip them.
		for _, b := range d[c.minSizeBytes-64 : c.minSizeBytes] {
			previousChunk.hash = (previousChunk.hash << 1) + gear[b]
		}
		previousChunk.end = c.minSizeBytes
		currentChunk = previousChunk
		oldChunks = c.chunks[:0]
	}

	for {
		// Start hashing data where the previous call left off. Stop
		// hashing when the current chunk becomes minSizeBytes in
		// size, as this requires us to insert a new chunk.
		hashRegion := d[currentChunk.end:]
		if m := c.minSizeBytes - (currentChunk.end - previousChunk.end); len(hashRegion) > m {
			hashRegion = hashRegion[:m]
		}
		if len(hashRegion) == 0 {
			if currentChunk.end-previousChunk.end == c.minSizeBytes {
				oldChunks = append(oldChunks, previousChunk)
				previousChunk = currentChunk
				continue
			}

			// Processed maxSizeBytes. Return the first chunk.
			c.chunks = append(oldChunks, previousChunk, currentChunk)
			return d[:c.chunks[0].end], nil
		}

		for i, b := range hashRegion {
			currentChunk.hash = (currentChunk.hash << 1) + gear[b]
			if currentChunk.hash > previousChunk.hash {
				// A cutting point has been found that is more
				// favorable than the previous one. Collapse
				// the current chunk into previous ones.
				for len(oldChunks) > 0 && currentChunk.hash > oldChunks[len(oldChunks)-1].hash {
					oldChunks = oldChunks[:len(oldChunks)-1]
				}
				previousChunk = chunk{
					hash: currentChunk.hash,
					end:  currentChunk.end + i + 1,
				}
			}
		}
		currentChunk.end += len(hashRegion)
	}
}
