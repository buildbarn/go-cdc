package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
)

func main() {
	existingSamples, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	samples := make([]uint64, 0, len(existingSamples)/8+1)
	for len(existingSamples) >= 8 {
		samples = append(samples, binary.LittleEndian.Uint64(existingSamples))
		existingSamples = existingSamples[8:]
	}

	s := uint64(0)
	for i := len(samples) - 1; i >= 0; i-- {
		s += samples[i]
		if samples[i] != 0 {
			samples[i] = s
		}
	}
	samplesCount := float64(samples[0])
	for i, s := range samples {
		if s != 0 {
			fmt.Printf("%d,%x\n", i, float64(s)/samplesCount)
		}
	}
}
