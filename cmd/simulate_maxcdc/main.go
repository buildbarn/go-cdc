package main

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"runtime"

	"golang.org/x/sync/semaphore"
)

func main() {
	const buckets = 1000
	os.MkdirAll(fmt.Sprintf("samples/%d", buckets), 0o777)

	// Shuffle the order in which we simulate each possible minSize.
	// This allows us to simulate all cases, even if we were to
	// interrupt and restart this tool.
	var minSizes [buckets - 1]int
	for i := 0; i < len(minSizes); i++ {
		minSizes[i] = i + 1
	}
	rand.Shuffle(len(minSizes), func(i, j int) {
		minSizes[i], minSizes[j] = minSizes[j], minSizes[i]
	})

	concurrency := semaphore.NewWeighted(int64(runtime.NumCPU()))
	for {
		for _, minSize := range minSizes {
			concurrency.Acquire(context.Background(), 1)
			go func() {
				defer concurrency.Release(1)

				var seed [32]byte
				crypto_rand.Read(seed[:])
				rng := rand.NewChaCha8(seed)

				// Perform simulation.
				var hashes [buckets]uint64
				hashesLength := 0
				var samples [buckets]uint64
				for range 10000000 {
					for hashesLength < len(hashes) {
						hashes[hashesLength] = rng.Uint64()
						hashesLength++
					}

					bestIndex := 0
					bestHash := hashes[bestIndex]
					for i := 1; i < buckets; i++ {
						if hash := hashes[i]; bestHash < hash {
							bestIndex = i
							bestHash = hash
						}
					}
					samples[bestIndex]++
					if start := bestIndex + minSize; start >= len(hashes) {
						hashesLength = 0
					} else {
						hashesLength = copy(hashes[:], hashes[bestIndex+minSize:])
					}
				}

				// Reload existing sample counts.
				samplesFile := fmt.Sprintf("samples/%d/%d", buckets, minSize)
				if existingSamples, _ := os.ReadFile(samplesFile); len(existingSamples) == 8*buckets {
					for i := 0; i < buckets; i++ {
						samples[i] += binary.LittleEndian.Uint64(existingSamples[8*i:])
					}
				}

				// Write new sample counts to disk.
				var newSamples [8 * buckets]byte
				for i := 0; i < buckets; i++ {
					binary.LittleEndian.PutUint64(newSamples[8*i:], samples[i])
				}
				if err := os.WriteFile(samplesFile+".tmp", newSamples[:], 0o666); err != nil {
					log.Fatal(err)
				}
				if err := os.Rename(samplesFile+".tmp", samplesFile); err != nil {
					log.Fatal(err)
				}
			}()
		}
	}
}
