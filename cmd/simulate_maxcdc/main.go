package main

import (
	"context"
	crypto_rand "crypto/rand"
	"fmt"
	"math"
	"math/rand/v2"
	"runtime"
	"sync"

	"golang.org/x/sync/semaphore"
)

func main() {
	var wg sync.WaitGroup
	concurrency := semaphore.NewWeighted(int64(runtime.NumCPU()))

	const buckets = 1000
	for minSizeIter := 1; minSizeIter < buckets; minSizeIter++ {
		minSize := minSizeIter

		concurrency.Acquire(context.Background(), 1)
		wg.Add(1)
		go func() {
			defer concurrency.Release(1)
			defer wg.Done()

			var seed [32]byte
			crypto_rand.Read(seed[:])
			rng := rand.NewChaCha8(seed)

			var samples [buckets]int
			const iterations = 1000000
			totalSize := 0
			uncoveredSize := 0
			var hashes [buckets]uint64
			hashesLength := 0
			for range iterations {
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
				totalSize += minSize + bestIndex

				if start := bestIndex + minSize; start >= len(hashes) {
					hashesLength = 0
					uncoveredSize += start - len(hashes)
				} else {
					hashesLength = copy(hashes[:], hashes[bestIndex+minSize:])
				}
			}

			maxSize := minSize + buckets - 1
			r := float64(maxSize) / float64(minSize)

			plateauSamples := samples[len(samples)-minSize:]
			plateauTotalSamples := 0
			for _, s := range plateauSamples {
				plateauTotalSamples += s
			}
			realPlateauProbability := float64(plateauTotalSamples) / float64(len(plateauSamples)) / float64(samples[0])
			expectedPlateauProbability := (2.289 + 0.577*r - 3.053*r*r) / (1 - 2.472*r - 1.179*r*r)

			realSkew := (float64(totalSize)/iterations)/(float64(minSize+maxSize)/2) - 1
			expectedSkew := 0.1449 - 1.162/math.Pow(r+3.015, 1.274)

			realCoverage := float64(totalSize-uncoveredSize) / float64(totalSize)
			expectedCoverage := math.Pow(r, 2.75) / (math.Pow(r, 2.75) + math.Pow(2, 2.75-1))

			fmt.Printf(
				"%d,%d,%f,%f,%f,%f,%f,%f,%f,%f,%f,%f\n",
				minSize,
				maxSize,
				r,
				realPlateauProbability,
				expectedPlateauProbability,
				realPlateauProbability-expectedPlateauProbability,
				realSkew,
				expectedSkew,
				realSkew-expectedSkew,
				realCoverage,
				expectedCoverage,
				realCoverage-expectedCoverage,
			)
		}()
	}
	wg.Wait()
}
