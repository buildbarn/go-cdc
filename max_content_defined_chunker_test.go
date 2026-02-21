package cdc_test

import (
	"bufio"
	"bytes"
	"io"
	"math/rand"
	"testing"

	"github.com/buildbarn/go-cdc"
	"github.com/stretchr/testify/require"
)

func TestMaxContentDefinedChunker(t *testing.T) {
	// Test that MaxContentDefinedChunker behaves the same way as
	// SimpleMaxContentDefinedChunker.
	seed := rand.Int63()
	r1 := rand.New(rand.NewSource(seed))
	r2 := rand.New(rand.NewSource(seed))

	for i := 0; i < 1000; i++ {
		chunker1 := cdc.NewSimpleMaxContentDefinedChunker(
			bufio.NewReaderSize(io.LimitReader(r1, 1024*1024), 64*1024),
			&cdc.FastContentDefinedChunkerGearTable,
			/* minSizeBytes = */ 2*1024,
			/* maxSizeBytes = */ 16*1024,
		)
		chunker2 := cdc.NewMaxContentDefinedChunker(
			bufio.NewReaderSize(io.LimitReader(r2, 1024*1024), 64*1024),
			&cdc.FastContentDefinedChunkerGearTable,
			/* minSizeBytes = */ 2*1024,
			/* maxSizeBytes = */ 16*1024,
		)

		for totalRead := 0; totalRead < 1024*1024; {
			chunk1, err1 := chunker1.ReadNextChunk()
			require.NoError(t, err1)
			require.LessOrEqual(t, 2*1024, len(chunk1))
			require.GreaterOrEqual(t, 16*1024, len(chunk1))

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
}

func FuzzMaxContentDefinedChunker(f *testing.F) {
	f.Fuzz(func(t *testing.T, gearSeed []byte, minSizeBytes, maxSizeBytes int, data []byte) {
		if minSizeBytes < 64 || maxSizeBytes < minSizeBytes {
			return
		}

		gearTable := cdc.NewSeededGearTable(gearSeed)
		chunker1 := cdc.NewSimpleMaxContentDefinedChunker(
			bufio.NewReader(bytes.NewBuffer(data)),
			gearTable,
			minSizeBytes,
			maxSizeBytes,
		)
		chunker2 := cdc.NewMaxContentDefinedChunker(
			bufio.NewReader(bytes.NewBuffer(data)),
			gearTable,
			minSizeBytes,
			maxSizeBytes,
		)

		for totalRead := 0; totalRead < len(data); {
			chunk1, err1 := chunker1.ReadNextChunk()
			require.NoError(t, err1)
			require.LessOrEqual(t, min(minSizeBytes, len(data)), len(chunk1))
			require.GreaterOrEqual(t, max(maxSizeBytes, 2*minSizeBytes), len(chunk1))

			chunk2, err2 := chunker2.ReadNextChunk()
			require.NoError(t, err2)
			require.Equal(t, chunk1, chunk2)
			totalRead += len(chunk1)
		}

		_, err1 := chunker1.ReadNextChunk()
		require.Equal(t, io.EOF, err1)
		_, err2 := chunker2.ReadNextChunk()
		require.Equal(t, io.EOF, err2)
	})
}
