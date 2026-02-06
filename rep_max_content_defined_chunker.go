package cdc

import (
	"bufio"
	"io"
	"slices"
)

type repMaxContentDefinedChunker struct {
	r             *bufio.Reader
	minSizeBytes  int
	peekSizeBytes int

	chunks []chunk
}

// NewRepMaxContentDefinedChunker returns a content defined chunker that
// expands upon MaxCDC, in that it repeatedly applies the chunking
// process until chunks are [minSizeBytes, 2*minSizeBytes) in size.
//
// Like MaxCDC, this algorithm takes a parameter that controls the
// amount of data that is read ahead. While MaxCDC uses it to control
// the maximum chunk size, in this algorithm it only denotes the quality
// of the chunking that is performed (i.e., the horizon size). Setting
// it to zero leads to uniform chunking of minSizeBytes, while setting
// it to a positive value n means that an optimal point within offsets
// [minSizeBytes, minSizeBytes+n] will always be respected.
//
// While MaxCDC performs poorly if the ratio between the maximum and
// minimum chunk size becomes too large, the horizon size can be
// increased freely without reducing quality. However, there will be
// diminishing returns.
//
// It has been observed that this algorithm provides an almost identical
// rate of deduplication as MaxCDC. The advantage of this algorithm over
// MaxCDC is that for a given input it is trivial to check whether it is
// already chunked, purely looking at its size.
func NewRepMaxContentDefinedChunker(r io.Reader, bufferSizeBytes, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &repMaxContentDefinedChunker{
		r:             bufio.NewReaderSize(r, bufferSizeBytes),
		minSizeBytes:  minSizeBytes,
		peekSizeBytes: 2*minSizeBytes + horizonSizeBytes,

		chunks: make([]chunk, 1),
	}
}

// addChunkAndDiscardExtraneous appends a newly observed cutting point
// to the list of potential cutting points.
//
// The end of the list may still contain extraneous potential cutting
// points that are situated close to each other. If the distance between
// the last potential cutting point and the newly observed cutting point
// is at least minSizeBytes, then we can clean up the extraneous
// potential cutting points by selecting the best one.
func (c *repMaxContentDefinedChunker) addChunkAndDiscardExtraneous(oldChunks []chunk, newChunk chunk) []chunk {
	if len(oldChunks) > 0 && newChunk.end-oldChunks[len(oldChunks)-1].end >= c.minSizeBytes {
		for i := len(oldChunks); i >= 2; i-- {
			if oldChunks[i-1].end-oldChunks[i-2].end < c.minSizeBytes {
				oldChunks[i-2] = oldChunks[i-1]
			}
		}
		oldChunks = slices.Compact(oldChunks)
	}
	return append(oldChunks, newChunk)
}

func (c *repMaxContentDefinedChunker) ReadNextChunk() ([]byte, error) {
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
	d, err := c.r.Peek(c.peekSizeBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(d) < 2*c.minSizeBytes {
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
		// This is the very first chunk. We know that the first
		// minSizeBytes positions can't contain a cut. Skip them.
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
				oldChunks = c.addChunkAndDiscardExtraneous(oldChunks, previousChunk)
				previousChunk = currentChunk
				continue
			}

			// Processed the full horizon. Return the first chunk.
			//
			// It may be the case that the start of the list
			// still contains extraneous cutting points. We
			// need to work our way backwards through the
			// list to determine how many of these
			// extraneous cutting points we should ignore.
			oldChunks = append(oldChunks, previousChunk)
			ignoreIndex := len(oldChunks) - 1
			for i := len(oldChunks); i > 1; i-- {
				if oldChunks[ignoreIndex].hash < oldChunks[i-2].hash || oldChunks[ignoreIndex].end-oldChunks[i-2].end >= c.minSizeBytes {
					ignoreIndex = i - 2
				}
			}
			if ignoreIndex > 0 {
				oldChunks = append(oldChunks[:0], oldChunks[ignoreIndex:]...)
			}
			c.chunks = append(oldChunks, currentChunk)
			return d[:c.chunks[0].end], nil
		}

		for i, b := range hashRegion {
			currentChunk.hash = (currentChunk.hash << 1) + gear[b]
			if currentChunk.hash > previousChunk.hash && currentChunk.end-previousChunk.end < c.minSizeBytes {
				// A cutting point has been found that
				// is more favorable than the previous
				// one. This doesn't mean we can't
				// discard the previous one just yet, as
				// we may need to use it if it turns out
				// an even more favorable cutting point
				// is located nearby.
				oldChunks = append(oldChunks, previousChunk)
				previousChunk = chunk{
					hash: currentChunk.hash,
					end:  currentChunk.end + i + 1,
				}
			}
		}
		currentChunk.end += len(hashRegion)
	}
}
