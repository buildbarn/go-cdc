package cdc

import (
	"io"
	"slices"
)

type repMaxContentDefinedChunker struct {
	gearTable                          *GearTable
	minSizeBytes                       int
	peekSizeBytes                      int
	supportsDiscardUpToGuaranteedChunk bool
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
func NewRepMaxContentDefinedChunker(gearTable *GearTable, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &repMaxContentDefinedChunker{
		gearTable:                          gearTable,
		minSizeBytes:                       minSizeBytes,
		peekSizeBytes:                      2*minSizeBytes + horizonSizeBytes,
		supportsDiscardUpToGuaranteedChunk: horizonSizeBytes >= 2*(minSizeBytes-1),
	}
}

func (c *repMaxContentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &repMaxChunkReader{
		contentDefinedChunker: c,
		peeker:                peeker,
		completeChunks:        make([]int, 0, c.peekSizeBytes/c.minSizeBytes),
		// Even though this list can grow to become proportional
		// to the size of the horizon, this is highly unlikely.
		// As we progress, it becomes increasingly harder to
		// find even more preferable cutting points within the
		// minimum chunk size. Allocating space for 32 cutting
		// points covers virtually all inputs.
		incompleteChunks: make([]int, 0, 32),
	}
}

func (c *repMaxContentDefinedChunker) SupportsDiscardUpToGuaranteedChunk() bool {
	return c.supportsDiscardUpToGuaranteedChunk
}

func (c *repMaxContentDefinedChunker) DiscardUpToGuaranteedChunk(peeker Peeker) error {
	if !c.supportsDiscardUpToGuaranteedChunk {
		panic("Horizon size is too small to permit discarding up to a guaranteed chunk")
	}

	// We need to keep continuous track of whether the current
	// candidate point has minSizeBytes-1 points before it at which
	// the rolling hash value is lower than the candidate's value.
	// Doing this accurately requires us to either recompute hashes
	// or allocate O(minSizeBytes) amount of memory.
	//
	// Instead of doing this, we divide the input into regions that
	// are minSizeBytes-1 in size. We only store the best hash value
	// for each region. This means that the point that ends up
	// getting selected actually has somewhere between
	// [minSizeBytes-1, 2*(minSizeBytes-1)) points before it with a
	// lower hash value. This makes this algorithm more picky than
	// necessary, but that's all right.
	//
	// The initial values of bestHashNext and bytesUntilNextRegion
	// are intentionally chosen so that the final byte of the
	// initial rolling hash window denotes the start of a new
	// region.
	gear := &c.gearTable.values
	var hash uint64
	bestHashCurrent := ^uint64(0)
	bestHashNext := ^uint64(0)
	bytesUntilNextRegion := gearHashWindowSizeBytes - 1
CheckPointsAfterCandidate:
	for {
		// The current offset can only be a valid cutting point
		// if there are at least minSizeBytes after it. However,
		// we only need to compare the candidate's rolling hash
		// value against the minSizeBytes-1 points after it. We
		// therefore peek minSizeBytes, but only process
		// minSizeBytes-1.
		d, err := peeker.Peek(c.minSizeBytes)
		if err != nil && err != io.EOF {
			return err
		}
		if len(d) < c.minSizeBytes {
			return io.EOF
		}
		hashRegion := d[:c.minSizeBytes-1]

		// See if there are any points within the trailing part of
		// the current region having a higher rolling hash value.
		for i, b := range hashRegion[:bytesUntilNextRegion] {
			hash = (hash << 1) + gear[b]
			if bestHashNext < hash {
				bestHashNext = hash
				if bestHashCurrent < bestHashNext {
					// Found a better candidate. Restart.
					if _, err := peeker.Discard(i + 1); err != nil {
						return err
					}
					bytesUntilNextRegion -= i + 1
					continue CheckPointsAfterCandidate
				}
			}
		}

		// End of current region. Transition to the next region
		// and process the first byte.
		hash = (hash << 1) + gear[hashRegion[bytesUntilNextRegion]]
		bestHashPrevious := bestHashCurrent
		bestHashCurrent, bestHashNext = bestHashNext, hash
		if bestHashPrevious >= bestHashCurrent || bestHashCurrent < bestHashNext {
			// First byte of the next region is a better
			// candidate, or the current candidate isn't
			// eligible anyway. Restart.
			if _, err := peeker.Discard(bytesUntilNextRegion + 1); err != nil {
				return err
			}
			bytesUntilNextRegion = c.minSizeBytes - 2
			continue CheckPointsAfterCandidate
		}

		// See if there are any points within the leading part of
		// the next region having a higher rolling hash value.
		for i, b := range hashRegion[bytesUntilNextRegion+1:] {
			hash = (hash << 1) + gear[b]
			if bestHashNext < hash {
				bestHashNext = hash
				if bestHashCurrent < bestHashNext {
					// Found a better candidate. Restart.
					if _, err := peeker.Discard(bytesUntilNextRegion + i + 2); err != nil {
						return err
					}
					bytesUntilNextRegion = c.minSizeBytes - i - 3
					continue CheckPointsAfterCandidate
				}
			}
		}

		// Current candidate is eligible, and the minSizeBytes-1
		// after it don't have a higher rolling hash value.
		return nil
	}
}

func (c *repMaxContentDefinedChunker) GetMaximumPeekSizeBytes() int {
	return c.peekSizeBytes
}

type repMaxChunkReader struct {
	contentDefinedChunker *repMaxContentDefinedChunker
	peeker                Peeker

	// The size of the previous chunk returned by ReadNextChunk().
	// This amount of data will be discarded from the input stream
	// at the start of the next call to ReadNextChunk().
	previousChunkSizeBytes int

	// List of chunks for which no future data can influence their
	// length. For each chunk, its size is stored. Chunks are stored
	// in reverse order, so that they can be popped from the end.
	completeChunks []int

	// List of cutting points that will determine the length of
	// future chunks. The hashes at the positions of the cutting
	// points in this list will be strictly monotonically
	// increasing.
	//
	// Cutting points are addressed relative to the first eligible
	// position at which they may be placed (i.e., the end of the
	// last complete chunk, plus the minimum chunk size). This means
	// that the first entry is always equal to zero.
	incompleteChunks []int

	// The rolling hash value corresponding to the position up to
	// where input data has been processed.
	currentHash uint64

	// The rolling hash value corresponding to the position of last
	// incomplete chunk. Any new incomplete chunk must have a hash
	// value that is higher than this one.
	bestHash uint64
}

func (r *repMaxChunkReader) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := r.peeker.Discard(r.previousChunkSizeBytes)
	r.previousChunkSizeBytes -= discardedSizeBytes
	if err != nil {
		return nil, err
	}

	// If the previous iteration yielded multiple chunks, we can
	// return them without peeking the full horizon. Doing so allows
	// us to discard data as aggressively as possible. This reduces
	// the amount of data that needs to be retained (copied) when
	// the read buffer is refilled.
	completeChunks := r.completeChunks
	if len(completeChunks) > 0 {
		firstChunk := completeChunks[len(completeChunks)-1]
		d, err := r.peeker.Peek(firstChunk)
		if err != nil {
			return nil, err
		}
		r.previousChunkSizeBytes = firstChunk
		r.completeChunks = completeChunks[:len(completeChunks)-1]
		return d, nil
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

	// Extract the final incomplete chunk from the stack, as it
	// denotes where the previous call stopped hashing the input.
	gear := &c.gearTable.values
	var oldChunks []int
	var currentChunk int
	var currentHash uint64
	var bestHash uint64
	if len(r.incompleteChunks) >= 2 {
		oldChunks = r.incompleteChunks[:len(r.incompleteChunks)-1]
		currentChunk = r.incompleteChunks[len(r.incompleteChunks)-1]
		currentHash = r.currentHash
		bestHash = r.bestHash
	} else {
		// This is the very first chunk. We know that the first
		// minSizeBytes positions can't contain a cut. Skip them.
		oldChunks = append(r.incompleteChunks[:0], 0)
		for _, b := range d[c.minSizeBytes-gearHashWindowSizeBytes : c.minSizeBytes] {
			currentHash = (currentHash << 1) + gear[b]
		}
		bestHash = currentHash
	}

	uncompletedRegion := d[c.minSizeBytes+currentChunk:]
	for {
		// Start hashing data where the previous call left off.
		// Stop hashing before the distance between two
		// consecutive potential cutting points becomes
		// minSizeBytes in size, as this allows us to complete a
		// chunk.
		hashRegion := uncompletedRegion
		originalOldChunksCount := -1
		if bytesBeforeMinChunkSize := oldChunks[len(oldChunks)-1] + c.minSizeBytes - 1 - currentChunk; len(hashRegion) > bytesBeforeMinChunkSize {
			hashRegion = hashRegion[:bytesBeforeMinChunkSize]
			originalOldChunksCount = len(oldChunks)
		} else if len(hashRegion) == 0 {
			break
		}

		// Preserve all offsets at which the hash increases.
		// The loop is unrolled manually, as the Go compiler
		// does not do it. Eight was empirically determined to
		// give good performance.
		i := 0
		for ; i+8 <= len(hashRegion); i += 8 {
			b := [8]byte(hashRegion[i : i+8])
			s := gear[b[0]]
			h := (currentHash << 1) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+1)
			}
			s = (s << 1) + gear[b[1]]
			h = (currentHash << 2) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+2)
			}
			s = (s << 1) + gear[b[2]]
			h = (currentHash << 3) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+3)
			}
			s = (s << 1) + gear[b[3]]
			h = (currentHash << 4) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+4)
			}
			s = (s << 1) + gear[b[4]]
			h = (currentHash << 5) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+5)
			}
			s = (s << 1) + gear[b[5]]
			h = (currentHash << 6) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+6)
			}
			s = (s << 1) + gear[b[6]]
			h = (currentHash << 7) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+7)
			}
			s = (s << 1) + gear[b[7]]
			h = (currentHash << 8) + s
			if bestHash < h {
				bestHash = h
				oldChunks = append(oldChunks, currentChunk+i+8)
			}
			currentHash = h
		}
		for ; i < len(hashRegion); i++ {
			currentHash = (currentHash << 1) + gear[hashRegion[i]]
			if bestHash < currentHash {
				bestHash = currentHash
				oldChunks = append(oldChunks, currentChunk+i+1)
			}
		}

		if len(oldChunks) == originalOldChunksCount {
			// The loop above did not yield any new cutting
			// points, and the next byte is minSizeBytes
			// away from the last cutting point. This means
			// we can complete all chunks up to this point.
			previousCompleteChunksCount := len(completeChunks)
			nextChunk := oldChunks[len(oldChunks)-1]
			for i := len(oldChunks) - 3; nextChunk >= c.minSizeBytes; i-- {
				chunk := oldChunks[i]
				if sizeBytes := nextChunk - chunk; sizeBytes >= c.minSizeBytes {
					completeChunks = append(completeChunks, sizeBytes)
					nextChunk = chunk
					i--
				}
			}
			completeChunks = append(completeChunks, c.minSizeBytes+nextChunk)
			slices.Reverse(completeChunks[previousCompleteChunksCount:])

			oldChunks = oldChunks[:1]
			currentChunk = 0
			currentHash = (currentHash << 1) + gear[uncompletedRegion[len(hashRegion)]]
			bestHash = currentHash
			uncompletedRegion = uncompletedRegion[len(hashRegion)+1:]
		} else {
			currentChunk += len(hashRegion)
			uncompletedRegion = uncompletedRegion[len(hashRegion):]
		}
	}

	// Processed the full horizon. Return the first chunk.
	incompleteChunks := append(oldChunks, currentChunk)
	var firstChunk int
	if len(completeChunks) > 0 {
		slices.Reverse(completeChunks)
		firstChunk = completeChunks[len(completeChunks)-1]
		completeChunks = completeChunks[:len(completeChunks)-1]
	} else {
		// The process above did not yield any complete chunks,
		// either because we reached the end of the file or the
		// horizon size wasn't large enough.
		//
		// Ensure that we pick a cutting point respecting the
		// maximum chunk size, that still allows us to pick the
		// most optimal cutting point in the horizon later on.
		firstChunkIndex := len(incompleteChunks) - 2
		for maxChunk, i := incompleteChunks[firstChunkIndex]-c.minSizeBytes, firstChunkIndex-2; maxChunk >= 0; i-- {
			if chunk := incompleteChunks[i]; chunk <= maxChunk {
				firstChunkIndex = i
				maxChunk = chunk - c.minSizeBytes
				i--
			}
		}
		firstChunk = c.minSizeBytes + incompleteChunks[firstChunkIndex]

		// There will be potential cutting points after the
		// selected one that are no longer eligible, as those
		// would violate the minimum chunk size. These should be
		// removed from the list.
		reusableChunkIndex := firstChunkIndex + 1
		for {
			if offsetInSecondChunk := incompleteChunks[reusableChunkIndex] - firstChunk; offsetInSecondChunk >= 0 {
				// This cutting point and the ones after
				// it should be kept.
				for i := reusableChunkIndex; i < len(incompleteChunks); i++ {
					incompleteChunks[i] -= firstChunk
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
					secondChunkRecomputedRegion := d[firstChunk:][:c.minSizeBytes+offsetInSecondChunk-1]
					var currentRecomputedHash uint64
					for _, b := range secondChunkRecomputedRegion[c.minSizeBytes-gearHashWindowSizeBytes : c.minSizeBytes] {
						currentRecomputedHash = (currentRecomputedHash << 1) + gear[b]
					}
					incompleteChunks[0] = 0
					bestRecomputedHash := currentRecomputedHash
					recomputedChunkIndex := 1
					originalChunksCount := len(incompleteChunks)
					for i, b := range secondChunkRecomputedRegion[c.minSizeBytes:] {
						currentRecomputedHash = (currentRecomputedHash << 1) + gear[b]
						if bestRecomputedHash < currentRecomputedHash {
							bestRecomputedHash = currentRecomputedHash
							recomputedChunk := i + 1
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
	r.previousChunkSizeBytes = firstChunk
	r.completeChunks = completeChunks
	r.incompleteChunks = incompleteChunks
	r.currentHash = currentHash
	r.bestHash = bestHash
	return d[:firstChunk], nil
}
