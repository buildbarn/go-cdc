package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"time"

	"github.com/buildbarn/go-cdc"
)

func main() {
	useFastCDC := false
	sizesCount := 113
	if useFastCDC {
		sizesCount = 8
	}
	for size := 0; size < sizesCount; size++ {
		var bestDuration time.Duration
		for iterationsLeft := 4; iterationsLeft >= 0; iterationsLeft-- {
			timeStart := time.Now()
			chunksCount := uint64(0)
			chunksTotalSizeBytes := uint64(0)
			for _, filename := range os.Args[1:] {
				f, err := os.Open(filename)
				if err != nil {
					log.Fatalf("Failed to open %#v: %s", filename, err)
				}
				br := bufio.NewReaderSize(f, 16*1024*1024)
				var r cdc.ContentDefinedChunker
				if useFastCDC {
					r = cdc.NewFastContentDefinedChunker(br, &cdc.FastContentDefinedChunkerGearTable, 512<<size)
				} else {
					minSizeBytes := int(math.Round(512 * 0.74759 * math.Pow(2, float64(size)/16)))
					r = cdc.NewRepMaxContentDefinedChunker(br, &cdc.FastContentDefinedChunkerGearTable, minSizeBytes, minSizeBytes*8)
				}
				for {
					chunk, err := r.ReadNextChunk()
					if err != nil {
						if err == io.EOF {
							break
						}
						log.Fatal(err)
					}
					chunksCount++
					chunksTotalSizeBytes += uint64(len(chunk))
				}
				f.Close()
			}
			if duration := time.Now().Sub(timeStart); bestDuration == 0 || bestDuration > duration {
				bestDuration = duration
			}
			if iterationsLeft == 0 {
				coveredTotalSizeBytes := chunksTotalSizeBytes
				if useFastCDC {
					coveredTotalSizeBytes -= chunksCount * uint64((128<<size)-1)
				}
				fmt.Printf(
					"%f,%f,%f\n",
					float64(chunksTotalSizeBytes)/float64(chunksCount),
					float64(chunksTotalSizeBytes)/bestDuration.Seconds(),
					float64(coveredTotalSizeBytes)/bestDuration.Seconds(),
				)
			}
		}
	}
}
