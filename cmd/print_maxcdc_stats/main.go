package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"os"
)

func main() {
	const buckets = 1000
	for minSize := buckets - 1; minSize >= 1; minSize-- {
		samplesFile := fmt.Sprintf("../simulate_maxcdc/samples/%d/%d", buckets, minSize)
		existingSamples, err := os.ReadFile(samplesFile)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.Fatal(err)
		}
		if len(existingSamples) != 8*buckets {
			log.Fatal("Invalid samples length")
		}
		var samples [buckets]uint64
		for i := 0; i < buckets; i++ {
			samples[i] = binary.LittleEndian.Uint64(existingSamples[8*i:])
		}

		maxSize := minSize + buckets - 1
		r := float64(maxSize) / float64(minSize)
		basicAverage := float64(minSize+maxSize) / 2

		iterations := uint64(0)
		totalSize := uint64(0)
		coveredSize := uint64(0)
		totalSizeSize := uint64(0)
		for i, s := range samples {
			chunkSize := uint64(minSize + i)
			iterations += s
			totalSize += s * chunkSize
			coveredSize += s * min(chunkSize, uint64(maxSize-minSize+1))
			totalSizeSize += s * chunkSize * chunkSize
		}

		plateauSamples := samples[len(samples)-minSize:]
		plateauTotalSamples := uint64(0)
		for _, s := range plateauSamples {
			plateauTotalSamples += s
		}
		realPlateauProbability := float64(plateauTotalSamples) / float64(len(plateauSamples)) / float64(samples[0])
		expectedPlateauProbability := (2.289 + 0.577*r - 3.053*r*r) / (1 - 2.472*r - 1.179*r*r)

		realSkewOut := (float64(totalSize)/float64(iterations))/basicAverage - 1
		expectedSkewOut := 0.1449 - 1.162/math.Pow(r+3.015, 1.274)

		realSkewModified := (float64(totalSizeSize)/float64(totalSize))/basicAverage - 1
		logR := math.Log(r)
		expectedSkewModified := (logR * (0.3973*logR - 0.1081)) / (logR*logR - 1.1511*logR + 3.5698)

		realCoverage := float64(coveredSize) / float64(totalSize)
		expectedCoverage := math.Pow(r, 2.75) / (math.Pow(r, 2.75) + math.Pow(2, 2.75-1))

		fmt.Printf(
			"%f, %f,%f,%f, %f,%f,%f, %f,%f,%f, %f,%f,%f\n",
			r,
			realPlateauProbability,
			expectedPlateauProbability,
			realPlateauProbability-expectedPlateauProbability,
			realSkewOut,
			expectedSkewOut,
			realSkewOut-expectedSkewOut,
			realSkewModified,
			expectedSkewModified,
			realSkewModified-expectedSkewModified,
			realCoverage,
			expectedCoverage,
			realCoverage-expectedCoverage,
		)
	}
}
