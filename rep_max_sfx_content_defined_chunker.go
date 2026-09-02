package cdc

import (
	"io"
	"math/bits"
	"slices"
)

// repMaxSfxIncompleteChunks represents a sequence of potential cutting
// points that are equidistant.
//
// In the case of RepMaxCDC, potential cutting points tend to be
// randomly distributed, only occurring with a relatively low
// probability. However, in the case of RepMaxSfxCDC it is easily
// possible to create long runs of closely spaced potential cutting
// points.
//
// Consider the sequence "...bababababac...". In that case each "b" and
// "c" is a potential cutting point, with the most preferable one being
// (the ones closest to) "c". For such a sequence, the `last` field will
// contain the offset of "c", and `period` will be set to 2.
type repMaxSfxIncompleteChunks struct {
	last   int
	period int
}

type repMaxSfxContentDefinedChunker struct {
	// TODO: Add support for synchronizing.
	nonSynchronizableContentDefinedChunker

	substitutionBox *SubstitutionBox
	minSizeBytes    int
	peekSizeBytes   int
}

// NewRepMaxSfxContentDefinedChunker returns a content defined chunker
// that, like RepMaxCDC, repeatedly applies the chunking process until
// chunks are [minSizeBytes, 2*minSizeBytes) in size.
//
// The difference between this algorithm and RepMaxCDC is that this
// algorithm does not use rolling hash functions. Instead, it selects
// cutting points by computing lexicographically maximum suffixes. By
// not depending on a rolling hash function with a fixed window size,
// this algorithm is capable of selecting cutting points in a more
// stable manner.
//
// A downside of using lexicographic comparisons is that it will cause
// poor performance if input data is sorted. In that case the start of
// each record may be treated as a potential cutting point, leading to
// excessive memory usage and poor chunking behavior. To address this,
// data is first passed through an S-box (substitution box).
func NewRepMaxSfxContentDefinedChunker(substitutionBox *SubstitutionBox, minSizeBytes, horizonSizeBytes int) ContentDefinedChunker {
	return &repMaxSfxContentDefinedChunker{
		substitutionBox: substitutionBox,
		minSizeBytes:    minSizeBytes,
		peekSizeBytes:   2*minSizeBytes + horizonSizeBytes,
	}
}

func (c *repMaxSfxContentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &repMaxSfxChunkReader{
		contentDefinedChunker: c,
		peeker:                peeker,
		completeChunks:        make([]int, 0, c.peekSizeBytes/c.minSizeBytes),
		oldChunks:             make([]repMaxSfxIncompleteChunks, 0, 64),
		firstBestChunks:       repMaxSfxIncompleteChunks{},
		currentChunk:          1,
	}
}

func (c *repMaxSfxContentDefinedChunker) GetMaximumPeekSizeBytes() int {
	return c.peekSizeBytes
}

type repMaxSfxChunkReader struct {
	contentDefinedChunker *repMaxSfxContentDefinedChunker
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
	// future chunks. The data after the positions of the cutting
	// points in this list will be strictly monotonically increasing
	// when compared lexicographically.
	//
	// Cutting points are addressed relative to the first eligible
	// position at which they may be placed (i.e., the end of the
	// last complete chunk, plus the minimum chunk size). This means
	// that the first entry is always equal to zero.
	oldChunks       []repMaxSfxIncompleteChunks
	firstBestChunks repMaxSfxIncompleteChunks

	// Last occurrence of the data associated with the best
	// potential cutting point. If the same data occurs at two
	// different positions, the algorithm should prefer cutting at
	// the first occurrence. However, in order to properly compute
	// periodicity, we must also keep track of the last occurrence.
	lastBestChunk int

	// Offset for which it's being checked whether it corresponds to
	// a potential cutting point.
	currentChunk int

	// Number of bytes past `currentChunk` that are equal to those
	// past `lastBestChunk`.
	matchLength int
}

func (r *repMaxSfxChunkReader) ReadNextChunk() ([]byte, error) {
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
	minSizeBytes := c.minSizeBytes
	if len(d) < 2*minSizeBytes {
		if len(d) == 0 {
			return nil, io.EOF
		}
		r.previousChunkSizeBytes = len(d)
		return d, nil
	}

	// Continue processing input where the previous call left off.
	uncompletedRegion := d[minSizeBytes:]
	oldChunks := r.oldChunks
	firstBestChunks := r.firstBestChunks
	lastBestChunk := r.lastBestChunk
	currentChunk := r.currentChunk
	matchLength := r.matchLength
	sbox := c.substitutionBox

	if matchLength != 0 {
		goto MatchSlow
	}

	// Optimize the case where we haven't yet matched any leading
	// bytes, and the region to scan is still sufficiently large.
	// In that case we may do some loop unrolling to compare eight
	// bytes at once, similar to what we do for plain RepMaxCDC.
MatchFirstBytes:
	if easyRegion := uncompletedRegion[currentChunk:min(len(uncompletedRegion), firstBestChunks.last+minSizeBytes)]; len(easyRegion) >= 7+8 {
		bestFirstBytes := uint64(sbox[uncompletedRegion[lastBestChunk]])<<56 |
			uint64(sbox[uncompletedRegion[lastBestChunk+1]])<<48 |
			uint64(sbox[uncompletedRegion[lastBestChunk+2]])<<40 |
			uint64(sbox[uncompletedRegion[lastBestChunk+3]])<<32 |
			uint64(sbox[uncompletedRegion[lastBestChunk+4]])<<24 |
			uint64(sbox[uncompletedRegion[lastBestChunk+5]])<<16 |
			uint64(sbox[uncompletedRegion[lastBestChunk+6]])<<8 |
			uint64(sbox[uncompletedRegion[lastBestChunk+7]])
		currentFirstBytes := uint64(sbox[easyRegion[0]])<<48 |
			uint64(sbox[easyRegion[1]])<<40 |
			uint64(sbox[easyRegion[2]])<<32 |
			uint64(sbox[easyRegion[3]])<<24 |
			uint64(sbox[easyRegion[4]])<<16 |
			uint64(sbox[easyRegion[5]])<<8 |
			uint64(sbox[easyRegion[6]])
		i := 7
		for ; i+8 <= len(easyRegion); i += 8 {
			b := [8]byte(easyRegion[i : i+8])
			newFirstBytes := uint64(sbox[b[0]])
			mergedFirstBytes := (currentFirstBytes << 8) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 7
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[1]])
			mergedFirstBytes = (currentFirstBytes << 16) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 6
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[2]])
			mergedFirstBytes = (currentFirstBytes << 24) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 5
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[3]])
			mergedFirstBytes = (currentFirstBytes << 32) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 4
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[4]])
			mergedFirstBytes = (currentFirstBytes << 40) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 3
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[5]])
			mergedFirstBytes = (currentFirstBytes << 48) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 2
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[6]])
			mergedFirstBytes = (currentFirstBytes << 56) | newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i - 1
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			newFirstBytes = (newFirstBytes << 8) | uint64(sbox[b[7]])
			mergedFirstBytes = newFirstBytes
			if mergedFirstBytes >= bestFirstBytes {
				currentChunk += i
				matchLength = bits.LeadingZeros64(mergedFirstBytes^bestFirstBytes) >> 3
				goto MatchSlow
			}
			currentFirstBytes = mergedFirstBytes
		}
		currentChunk += i - 7
		goto MatchSlow
	}

MatchSlow:
	for currentChunk+matchLength < len(uncompletedRegion) {
		if ca, cb := sbox[uncompletedRegion[lastBestChunk+matchLength]], sbox[uncompletedRegion[currentChunk+matchLength]]; ca > cb {
			// Current candidate is worse than what has
			// already been observed. Reset and match from
			// the beginning.
			currentChunk += matchLength + 1
			if currentChunk >= firstBestChunks.last+minSizeBytes {
				goto CompleteChunks
			}
			matchLength = 0
			goto MatchFirstBytes
		} else if ca < cb {
			// Current candidate is better than what has
			// already been observed. Store its offset.
			if distance := currentChunk - firstBestChunks.last; firstBestChunks.period == distance {
				firstBestChunks.last = currentChunk
			} else {
				oldChunks = append(oldChunks, firstBestChunks)
				firstBestChunks = repMaxSfxIncompleteChunks{
					last:   currentChunk,
					period: distance,
				}
			}

			if period := currentChunk - lastBestChunk; matchLength >= period {
				// The best potential cutting point is
				// followed by repeated data. This means
				// that we must register multiple
				// potential cutting points.
				oldMatchLength := matchLength
				matchLength %= period
				currentChunk += oldMatchLength - matchLength
				if matchLength == 0 {
					// "bbaabbaabbaac". The new best
					// potential cutting point is at "c".
					// The next byte to compare against
					// process should be the one after
					// "c".
					lastBestChunk = currentChunk
					currentChunk++
				} else {
					// "bbaabbaabbaabbc". Even though "c"
					// is the new best potential cutting
					// point, we must also create
					// potential cutting points for "bbc"
					// and "bc". This is why we pick
					// "bbc" for now.
					lastBestChunk = currentChunk - period
				}
				if firstBestChunks.last != lastBestChunk {
					if firstBestChunks.period == period {
						firstBestChunks.last = lastBestChunk
					} else {
						oldChunks = append(oldChunks, firstBestChunks)
						firstBestChunks = repMaxSfxIncompleteChunks{
							last:   lastBestChunk,
							period: period,
						}
					}
				}
				if matchLength == 0 {
					goto MatchFirstBytes
				}
			} else {
				lastBestChunk = currentChunk
				currentChunk++
				matchLength = 0
				goto MatchFirstBytes
			}
		} else {
			// Best candidate and current candidate share the
			// same prefix. Continue matching the next byte.
			matchLength++
			if matchLength == minSizeBytes {
				// We only want to compare up to minSizeBytes
				// of data, as otherwise we may be influenced
				// by data residing beyond the resulting
				// chunk.
				//
				// If we end up here, the best and current
				// candidate are equally good. As the
				// algorithm should prefer cutting at the
				// first occurrence of data, the current
				// candidate is not eligible. However, the
				// current candidate should still be respected
				// to make computation of the period work.
				period := currentChunk - lastBestChunk
				currentChunk += period
				if currentChunk >= firstBestChunks.last+minSizeBytes {
					goto CompleteChunks
				}
				lastBestChunk += period
				matchLength -= period
			}
		}
		continue

	CompleteChunks:
		// We've reached the point where none of the cutting
		// points obtained thus far can be further influenced by
		// data that follows. This means we can complete all
		// chunks up to this point.
		//
		// Perform a reverse pass against all obtained cutting
		// points to select ones that are at least minSizeBytes
		// apart.
		uncompletedRegion = uncompletedRegion[minSizeBytes+firstBestChunks.last:]
		previousCompleteChunksCount := len(completeChunks)
		for i := len(oldChunks) - 1; firstBestChunks.last >= minSizeBytes; i-- {
			previousChunk := oldChunks[i]
			for {
				maxNextChunk := firstBestChunks.last - minSizeBytes
				if previousChunk.last > maxNextChunk {
					break
				}
				chunk := previousChunk.last + (maxNextChunk-previousChunk.last)/firstBestChunks.period*firstBestChunks.period
				completeChunks = append(completeChunks, firstBestChunks.last-chunk)
				firstBestChunks.last = chunk
			}
			firstBestChunks.period = previousChunk.period
		}
		completeChunks = append(completeChunks, minSizeBytes+firstBestChunks.last)
		slices.Reverse(completeChunks[previousCompleteChunksCount:])

		// Reinitialize, so that we compute chunks following the
		// ones that were completed.
		oldChunks = oldChunks[:0]
		firstBestChunks = repMaxSfxIncompleteChunks{}
		lastBestChunk = 0
		currentChunk = 1
		matchLength = 0
		if len(uncompletedRegion) <= 1 {
			break
		}
		goto MatchFirstBytes
	}

	// Processed the full horizon. Return the first chunk.
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
		// maximum chunk size that still allows us to pick the
		// most optimal cutting point in the horizon later on.
		//
		// The process above doesn't just guarantee finding all
		// potential cutting points inside the horizon. It may
		// also discover ones in the minSizeBytes that were
		// peeked beyond it. In order behave consistency with
		// the simple algorithm, we first remove the excess
		// cutting points.
		for len(oldChunks) > 0 && oldChunks[len(oldChunks)-1].last >= len(d)-2*minSizeBytes {
			firstBestChunks = oldChunks[len(oldChunks)-1]
			oldChunks = oldChunks[:len(oldChunks)-1]
		}
		if firstBestChunks.last > len(d)-2*minSizeBytes {
			firstBestChunks.last -= (firstBestChunks.last - (len(d) - 2*minSizeBytes) + (firstBestChunks.period - 1)) / firstBestChunks.period * firstBestChunks.period
		}
		for i := len(oldChunks) - 1; firstBestChunks.last >= minSizeBytes; i-- {
			previousChunk := oldChunks[i]
			for {
				maxNextChunk := firstBestChunks.last - minSizeBytes
				if previousChunk.last > maxNextChunk {
					break
				}
				chunk := previousChunk.last + (maxNextChunk-previousChunk.last)/firstBestChunks.period*firstBestChunks.period
				firstBestChunks.last = chunk
			}
			firstBestChunks.period = previousChunk.period
		}

		// RepMaxCDC has logic for preserving cutting points,
		// only recomputing ones in a small region following the
		// chunk. We simply reinitialize at this point.
		//
		// TODO: Do we want to port over this logic? Given a
		// sufficiently large horizon, this should only occur
		// rarely.
		firstChunk = minSizeBytes + firstBestChunks.last
		oldChunks = oldChunks[:0]
		firstBestChunks = repMaxSfxIncompleteChunks{}
		lastBestChunk = 0
		currentChunk = 1
		matchLength = 0
	}
	r.previousChunkSizeBytes = firstChunk
	r.completeChunks = completeChunks
	r.oldChunks = oldChunks
	r.firstBestChunks = firstBestChunks
	r.lastBestChunk = lastBestChunk
	r.currentChunk = currentChunk
	r.matchLength = matchLength
	return d[:firstChunk], nil
}
