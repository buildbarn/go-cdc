package cdc

import (
	"io"
	"slices"
)

type repMaxContentDefinedChunker struct {
	r             Peeker
	minSizeBytes  int
	peekSizeBytes int

	// List of chunks for which no future data can influence their
	// length. The first element corresponds to the amount of data
	// belonging to the chunk that was returned by the previous call
	// to ReadNextChunk() that hasn't been discarded yet.
	completeChunks []int

	// List of cutting points that will determine the length of
	// future chunks. The hashes of the cutting points in this list
	// will be strictly monotonically increasing. Cutting points are
	// addressed relative to the first eligible position at which
	// they may be placed (i.e., the end of the last complete chunk,
	// plus the minimum chunk size).
	incompleteChunks []chunk
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
func NewRepMaxContentDefinedChunker(r Peeker, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &repMaxContentDefinedChunker{
		r:             r,
		minSizeBytes:  minSizeBytes,
		peekSizeBytes: 2*minSizeBytes + horizonSizeBytes,

		completeChunks: make([]int, 1, horizonSizeBytes/minSizeBytes+2),
	}
}

func (c *repMaxContentDefinedChunker) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := c.r.Discard(c.completeChunks[0])
	for i := range c.completeChunks {
		c.completeChunks[i] -= discardedSizeBytes
	}
	if err != nil {
		return nil, err
	}

	// If the previous iteration yielded multiple chunks, we can
	// return them without peeking the full horizon. Doing so allows
	// us to discard data as aggressively as possible. This reduces
	// the amount of data that needs to be retained (copied) when
	// the read buffer is refilled.
	if len(c.completeChunks) > 1 {
		d, err := c.r.Peek(c.completeChunks[1])
		if err != nil {
			return nil, err
		}
		c.completeChunks = append(c.completeChunks[:0], c.completeChunks[1:]...)
		return d, nil
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
		c.completeChunks = append(c.completeChunks[:0], len(d))
		c.incompleteChunks = c.incompleteChunks[:0]
		return d, nil
	}
	d = d[:len(d)-c.minSizeBytes]

	// Extract the final incomplete chunk from the stack, as it
	// denotes where the previous call stopped hashing the input.
	var oldChunks []chunk
	var currentChunk chunk
	if len(c.incompleteChunks) >= 2 {
		currentChunk = c.incompleteChunks[len(c.incompleteChunks)-1]
		oldChunks = c.incompleteChunks[:len(c.incompleteChunks)-1]
	} else {
		// This is the very first chunk. We know that the first
		// minSizeBytes positions can't contain a cut. Skip them.
		for _, b := range d[c.minSizeBytes-64 : c.minSizeBytes] {
			currentChunk.hash = (currentChunk.hash << 1) + gear[b]
		}
		oldChunks = append(oldChunks[:0], currentChunk)
	}

	for {
		// Start hashing data where the previous call left off.
		// Stop hashing before the distance between two
		// consecutive potential cutting points becomes
		// minSizeBytes in size, as this allows us to complete a
		// chunk.
		hashRegion := d[c.completeChunks[len(c.completeChunks)-1]+c.minSizeBytes+currentChunk.end:]
		originalOldChunksCount := -1
		var completingByte byte
		if bytesBeforeMinChunkSize := oldChunks[len(oldChunks)-1].end + c.minSizeBytes - 1 - currentChunk.end; len(hashRegion) > bytesBeforeMinChunkSize {
			completingByte = hashRegion[bytesBeforeMinChunkSize]
			hashRegion = hashRegion[:bytesBeforeMinChunkSize]
			originalOldChunksCount = len(oldChunks)
		} else if len(hashRegion) == 0 {
			break
		}

		// Preserve all offsets at which the hash increases.
		bestHash := oldChunks[len(oldChunks)-1].hash
		for i, b := range hashRegion {
			currentChunk.hash = (currentChunk.hash << 1) + gear[b]
			if bestHash < currentChunk.hash {
				bestHash = currentChunk.hash
				oldChunks = append(oldChunks, chunk{
					hash: currentChunk.hash,
					end:  currentChunk.end + i + 1,
				})
			}
		}

		if len(oldChunks) == originalOldChunksCount {
			// The loop above did not yield any new cutting
			// points, and the next byte is minSizeBytes
			// away from the last cutting point. This means
			// we can complete all chunks up to this point.
			previousCompleteChunksCount := len(c.completeChunks)
			firstNewCompleteChunkStart := c.completeChunks[len(c.completeChunks)-1] + c.minSizeBytes
			nextChunkEnd := oldChunks[len(oldChunks)-1].end
			c.completeChunks = append(c.completeChunks, firstNewCompleteChunkStart+nextChunkEnd)
			for i := len(oldChunks) - 2; i >= 0; i-- {
				currentChunkEnd := oldChunks[i].end
				if nextChunkEnd-currentChunkEnd >= c.minSizeBytes {
					c.completeChunks = append(c.completeChunks, firstNewCompleteChunkStart+currentChunkEnd)
					nextChunkEnd = currentChunkEnd
				}
			}
			slices.Reverse(c.completeChunks[previousCompleteChunksCount:])

			currentChunk = chunk{
				hash: (currentChunk.hash << 1) + gear[completingByte],
			}
			oldChunks = append(oldChunks[:0], currentChunk)
		} else {
			currentChunk.end += len(hashRegion)
		}
	}

	// Processed the full horizon. Return the first chunk.
	incompleteChunks := append(oldChunks, currentChunk)
	if len(c.completeChunks) == 1 {
		// The process above did not yield any complete chunks,
		// either because we reached the end of the file or the
		// horizon size wasn't large enough.
		//
		// Ensure that we pick a cutting point respecting the
		// maximum chunk size, that still allows us to pick the
		// most optimal cutting point in the horizon later on.
		firstChunkIndex := len(incompleteChunks) - 2
		for i := len(incompleteChunks) - 3; i >= 0; i-- {
			if incompleteChunks[firstChunkIndex].hash < incompleteChunks[i].hash || incompleteChunks[firstChunkIndex].end-incompleteChunks[i].end >= c.minSizeBytes {
				firstChunkIndex = i
			}
		}
		firstChunkEnd := c.minSizeBytes + incompleteChunks[firstChunkIndex].end
		firstChunkCompleteOffset := c.completeChunks[len(c.completeChunks)-1] + firstChunkEnd
		c.completeChunks = append(c.completeChunks, firstChunkCompleteOffset)

		// There will be potential cutting points after the
		// selected one that are no longer eligible, as those
		// would violate the minimum chunk size. These should be
		// removed from the list.
		reusableChunkIndex := firstChunkIndex + 1
		for {
			if offsetInSecondChunk := incompleteChunks[reusableChunkIndex].end - firstChunkEnd; offsetInSecondChunk >= 0 {
				// This cutting point and the ones after
				// it should be kept.
				for i := reusableChunkIndex; i < len(incompleteChunks); i++ {
					incompleteChunks[i].end -= firstChunkEnd
				}

				if offsetInSecondChunk == 0 {
					// There is no need to recompute any
					// cutting points.
					incompleteChunks = append(incompleteChunks[:0], incompleteChunks[reusableChunkIndex:]...)
				} else {
					// Because the first cutting point to
					// keep resides at an offset beyond
					// the minimum chunk size, we may have
					// glossed over potential cutting
					// points before it. Recompute these.
					//
					// This should only happen rarely,
					// especially if the horizon size is
					// sufficiently large.
					nextChunkStart := d[firstChunkCompleteOffset:][:c.minSizeBytes+offsetInSecondChunk-1]
					hash := uint64(0)
					for _, b := range nextChunkStart[c.minSizeBytes-64 : c.minSizeBytes] {
						hash = (hash << 1) + gear[b]
					}
					incompleteChunks[0] = chunk{hash: hash}
					bestHash := hash
					recomputedChunkIndex := 1
					originalChunksCount := len(incompleteChunks)
					for i, b := range nextChunkStart[c.minSizeBytes:] {
						hash = (hash << 1) + gear[b]
						if bestHash < hash {
							bestHash = hash
							recomputedChunk := chunk{
								hash: hash,
								end:  i + 1,
							}
							if recomputedChunkIndex < reusableChunkIndex {
								incompleteChunks[recomputedChunkIndex] = recomputedChunk
								recomputedChunkIndex++
							} else {
								incompleteChunks = append(incompleteChunks, recomputedChunk)
							}
						}
					}
					if recomputedChunkIndex < reusableChunkIndex {
						// Recomputing yielded fewer cutting points
						// than we had previously. Make the cutting
						// points contiguous again.
						incompleteChunks = append(incompleteChunks[:recomputedChunkIndex], incompleteChunks[reusableChunkIndex:]...)
					} else if len(incompleteChunks) > originalChunksCount {
						// Recomputing yielded more cutting points
						// than we had previously. The excess
						// cutting points were stored at the end.
						// Rotate them into place, so that the list
						// remains sorted.
						slices.Reverse(incompleteChunks[reusableChunkIndex:originalChunksCount])
						slices.Reverse(incompleteChunks[originalChunksCount:])
						slices.Reverse(incompleteChunks[reusableChunkIndex:])
					}
				}
				break
			}

			// The cutting point should be removed.
			reusableChunkIndex++
			if reusableChunkIndex == len(incompleteChunks) {
				incompleteChunks = incompleteChunks[:1]
				break
			}
		}
	}
	c.completeChunks = append(c.completeChunks[:0], c.completeChunks[1:]...)
	c.incompleteChunks = incompleteChunks
	return d[:c.completeChunks[0]], nil
}
