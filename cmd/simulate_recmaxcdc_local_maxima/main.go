package main

import (
	crypto_rand "crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"runtime"
)

func main() {
	for cpu := 0; cpu < runtime.NumCPU(); cpu++ {
		cpuNum := cpu
		go func() {
			for {
				var seed [32]byte
				crypto_rand.Read(seed[:])
				rng := rand.NewChaCha8(seed)

				var distanceSamples []uint64
				var betterHashesSamples []uint64
				const iterations = 1000000
				for i := 0; i < iterations; i++ {
					bestHash := rng.Uint64()
					distance := 0
					lowerHashCount := 0
					betterHashes := 0
					for {
						lowerHashCount++
						if lowerHashCount == 2048 {
							break
						}
						if hash := rng.Uint64(); bestHash < hash {
							bestHash = hash
							distance += lowerHashCount
							lowerHashCount = 0
							betterHashes++
						}
					}

					for len(distanceSamples) <= distance {
						distanceSamples = append(distanceSamples, 0)
					}
					distanceSamples[distance]++

					for len(betterHashesSamples) <= betterHashes {
						betterHashesSamples = append(betterHashesSamples, 0)
					}
					betterHashesSamples[betterHashes]++
				}

				updateSamplesFile := func(name string, samples *[]uint64) {
					existingSamples, _ := os.ReadFile(name)
					for idx := 0; len(existingSamples) >= 8; idx++ {
						for len(*samples) <= idx {
							*samples = append(*samples, 0)
						}
						(*samples)[idx] += binary.LittleEndian.Uint64(existingSamples)
						existingSamples = existingSamples[8:]
					}

					// Write new sample counts to disk.
					newSamples := make([]byte, 8*len(*samples))
					for i, s := range *samples {
						binary.LittleEndian.PutUint64(newSamples[8*i:], s)
					}
					tmpFile := fmt.Sprintf("%s.%d", name, cpuNum)
					if err := os.WriteFile(tmpFile, newSamples, 0o666); err != nil {
						log.Fatal(err)
					}
					if err := os.Rename(tmpFile, name); err != nil {
						log.Fatal(err)
					}
				}
				updateSamplesFile("distance", &distanceSamples)
				updateSamplesFile("better_hashes", &betterHashesSamples)
			}
		}()
	}
	select {}
}
