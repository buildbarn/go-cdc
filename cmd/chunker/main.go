package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/buildbarn/go-cdc"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal("Failed to open input file: ", err)
	}

	r := cdc.NewMaxContentDefinedChunker(f, 16*1024*1024, 2*1024, 16*1024)
	// r := cdc.NewFastContentDefinedChunker(f, 16*1024*1024)

	chunkCount := 0
	for {
		chunk, err := r.ReadNextChunk()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}
		chunkCount++
		h := sha256.Sum256(chunk)
		fmt.Printf("%s-%d\n", hex.EncodeToString(h[:]), len(chunk))
	}
	log.Printf("Created %d chunks", chunkCount)
}
