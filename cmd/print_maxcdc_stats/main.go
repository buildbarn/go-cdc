package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"

	goat_math "github.com/xaevman/goat/lib/math"

	"gonum.org/v1/gonum/optimize"
)

func computePadéApproximant(r float64, params []float64) float64 {
	return (params[0] + params[2]*r + params[4]*r*r) / (params[1] + params[3]*r + r*r)
}

func main() {
	const statsCount = 5
	type stat struct {
		r      float64
		values [statsCount]float64
	}
	var stats []stat

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
		plateauProbability := float64(plateauTotalSamples) / float64(len(plateauSamples)) / float64(samples[0])
		ratioOut := (float64(totalSize) / float64(iterations)) / basicAverage
		skewOut := ratioOut - 1
		ratioModified := (float64(totalSizeSize) / float64(totalSize)) / basicAverage
		skewModified := ratioModified - 1
		skewModifiedOverOut := ratioModified/ratioOut - 1
		coverage := float64(coveredSize) / float64(totalSize)

		stats = append(stats, stat{
			r: r,
			values: [...]float64{
				plateauProbability,
				skewOut,
				skewModified,
				skewModifiedOverOut,
				coverage,
			},
		})
	}

	var padéParams [][]float64
	for i := 0; i < statsCount; i++ {
		result, err := optimize.Minimize(
			optimize.Problem{
				Func: func(params []float64) float64 {
					var sum goat_math.KahanSum
					for _, s := range stats {
						e := computePadéApproximant(s.r, params) - s.values[i]
						sum.Add(e * e)
					}
					return sum.Sum()
				},
			},
			[]float64{
				0,
				0,
				0,
				0,
				stats[len(stats)-1].values[i],
			},
			nil,
			&optimize.NelderMead{},
		)
		if err != nil {
			log.Fatal(err)
			return
		}
		padéParams = append(padéParams, result.X)
	}

	for _, s := range stats {
		fmt.Printf("%f", s.r)
		for i := 0; i < statsCount; i++ {
			actual := s.values[i]
			expected := computePadéApproximant(s.r, padéParams[i])
			fmt.Printf(
				", %f,%f,%f",
				actual,
				expected,
				actual-expected,
			)

		}
		fmt.Printf("\n")
	}

	fmt.Println("---")
	for _, params := range padéParams {
		fmt.Printf(
			"f(r) = (%.3f + %.3f * r + %.3f * r * r) / (%.3f + %.3f * r + r * r)\n",
			params[0],
			params[2],
			params[4],
			params[1],
			params[3],
		)
	}
}
