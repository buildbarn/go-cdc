package cdc_test

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/buildbarn/go-cdc"
	"github.com/stretchr/testify/require"
)

func TestRepMaxContentDefinedChunker(t *testing.T) {
	// Test that RepMaxContentDefinedChunker behaves the same way as
	// SimpleRepMaxContentDefinedChunker.
	seed := rand.Int63()
	r1 := rand.New(rand.NewSource(seed))
	r2 := rand.New(rand.NewSource(seed))

	for horizonSizeBytes := 0; horizonSizeBytes <= 16*1024; horizonSizeBytes += 2 * 1024 {
		t.Run(fmt.Sprintf("Horizon=%d", horizonSizeBytes), func(t *testing.T) {
			for i := 0; i < 100; i++ {
				chunker1 := cdc.NewSimpleRepMaxContentDefinedChunker(
					bufio.NewReaderSize(io.LimitReader(r1, 1024*1024), 64*1024),
					/* minSizeBytes = */ 2*1024,
					horizonSizeBytes,
				)
				chunker2 := cdc.NewRepMaxContentDefinedChunker(
					bufio.NewReaderSize(io.LimitReader(r2, 1024*1024), 64*1024),
					/* minSizeBytes = */ 2*1024,
					horizonSizeBytes,
				)

				for totalRead := 0; totalRead < 1024*1024; {
					chunk1, err1 := chunker1.ReadNextChunk()
					require.NoError(t, err1)
					require.LessOrEqual(t, 2*1024, len(chunk1))
					require.Greater(t, 4*1024, len(chunk1))

					chunk2, err2 := chunker2.ReadNextChunk()
					require.NoError(t, err2)
					require.Equal(t, chunk1, chunk2)
					totalRead += len(chunk1)
				}

				_, err1 := chunker1.ReadNextChunk()
				require.Equal(t, io.EOF, err1)
				_, err2 := chunker2.ReadNextChunk()
				require.Equal(t, io.EOF, err2)
			}
		})
	}
}
