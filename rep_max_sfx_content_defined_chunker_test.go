package cdc_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/buildbarn/go-cdc"
	"github.com/stretchr/testify/require"
)

func TestRepMaxSfxContentDefinedChunkerNewChunkReader(t *testing.T) {
	t.Run("Static", func(t *testing.T) {
		for _, testCase := range []struct {
			minSizeBytes     int
			horizonSizeBytes int
			data             string
			cuts             []int
		}{
			{
				minSizeBytes:     2,
				horizonSizeBytes: 40,
				data:             "01210",
				cuts:             []int{2, 3},
			},
			{
				minSizeBytes:     2,
				horizonSizeBytes: 40,
				data:             "01234",
				cuts:             []int{3, 2},
			},
			{
				minSizeBytes:     2,
				horizonSizeBytes: 0,
				data:             "01234",
				cuts:             []int{2, 3},
			},
			{
				minSizeBytes:     3,
				horizonSizeBytes: 10,
				data:             "0000001",
				cuts:             []int{4, 3},
			},
			{
				minSizeBytes:     5,
				horizonSizeBytes: 23,
				data:             "0000000000100000",
				cuts:             []int{5, 5, 6},
			},
			{
				minSizeBytes:     5,
				horizonSizeBytes: 23,
				data:             "00000001000",
				cuts:             []int{6, 5},
			},
			{
				minSizeBytes:     2,
				horizonSizeBytes: 82,
				data:             "0000120",
				cuts:             []int{3, 2, 2},
			},
			{
				minSizeBytes:     15,
				horizonSizeBytes: 44,
				data:             "00000000000000000000000000000001",
				cuts:             []int{17, 15},
			},
			{
				minSizeBytes:     7,
				horizonSizeBytes: 17,
				data:             "000000011111111",
				cuts:             []int{7, 8},
			},
			{
				minSizeBytes:     7,
				horizonSizeBytes: 17,
				data:             "000000000000011110011",
				cuts:             []int{13, 8},
			},
			{
				minSizeBytes:     7,
				horizonSizeBytes: 17,
				data:             "000000022222220000001",
				cuts:             []int{7, 7, 7},
			},
			{
				minSizeBytes:     7,
				horizonSizeBytes: 17,
				data:             "000000011111111111111",
				cuts:             []int{7, 7, 7},
			},
			{
				minSizeBytes:     7,
				horizonSizeBytes: 17,
				data:             "00000000010002",
				cuts:             []int{7, 7},
			},
			{
				minSizeBytes:     2,
				horizonSizeBytes: 17,
				data:             "0000010",
				cuts:             []int{2, 3, 2},
			},
			{
				minSizeBytes:     2,
				horizonSizeBytes: 224,
				data:             "0000001",
				cuts:             []int{2, 3, 2},
			},
			{
				minSizeBytes:     4,
				horizonSizeBytes: 106,
				data:             "0000010101010110000000000",
				cuts:             []int{5, 4, 4, 4, 4, 4},
			},
			{
				minSizeBytes:     2,
				horizonSizeBytes: 3,
				data:             "00001120",
				cuts:             []int{3, 3, 2},
			},
			{
				minSizeBytes:     7,
				horizonSizeBytes: 17,
				data:             "00000000100100120000000",
				cuts:             []int{8, 7, 8},
			},
			{
				minSizeBytes:     10,
				horizonSizeBytes: 84,
				data:             "000000000011011011112000000",
				cuts:             []int{17, 10},
			},
			{
				minSizeBytes:     28,
				horizonSizeBytes: 29,
				data:             "0000000000000000000000000000100100100010010010001001001001001010000000000000000",
				cuts:             []int{51, 28},
			},
		} {
			t.Run(fmt.Sprintf("%d/%d/%s", testCase.minSizeBytes, testCase.horizonSizeBytes, testCase.data), func(t *testing.T) {
				chunker := cdc.NewRepMaxSfxContentDefinedChunker(
					&cdc.NoSubstitutionBox,
					testCase.minSizeBytes,
					testCase.horizonSizeBytes,
				)
				chunkReader := chunker.NewChunkReader(
					bufio.NewReader(bytes.NewBufferString(testCase.data)),
				)

				for _, cut := range testCase.cuts {
					chunk, err := chunkReader.ReadNextChunk()
					require.NoError(t, err)
					require.Len(t, chunk, cut)
				}

				_, err := chunkReader.ReadNextChunk()
				require.Equal(t, io.EOF, err)
			})
		}
	})

	t.Run("Random", func(t *testing.T) {
		// Test that RepMaxSfxContentDefinedChunker behaves the
		// same way as SimpleRepMaxSfxContentDefinedChunker.
		seed := rand.Int63()
		r1 := rand.New(rand.NewSource(seed))
		r2 := rand.New(rand.NewSource(seed))

		for horizonSizeBytes := 0; horizonSizeBytes <= 16*1024; horizonSizeBytes += 2 * 1024 {
			t.Run(fmt.Sprintf("Horizon=%d", horizonSizeBytes), func(t *testing.T) {
				chunker1 := cdc.NewSimpleRepMaxSfxContentDefinedChunker(
					/* minSizeBytes = */ 2*1024,
					horizonSizeBytes,
				)
				chunker2 := cdc.NewRepMaxSfxContentDefinedChunker(
					&cdc.NoSubstitutionBox,
					/* minSizeBytes = */ 2*1024,
					horizonSizeBytes,
				)

				for i := 0; i < 100; i++ {
					chunkReader1 := chunker1.NewChunkReader(
						bufio.NewReaderSize(io.LimitReader(r1, 1024*1024), 64*1024),
					)
					chunkReader2 := chunker2.NewChunkReader(
						bufio.NewReaderSize(io.LimitReader(r2, 1024*1024), 64*1024),
					)

					for totalRead := 0; totalRead < 1024*1024; {
						chunk1, err1 := chunkReader1.ReadNextChunk()
						require.NoError(t, err1)
						require.LessOrEqual(t, 2*1024, len(chunk1))
						require.Greater(t, 4*1024, len(chunk1))

						chunk2, err2 := chunkReader2.ReadNextChunk()
						require.NoError(t, err2)
						require.Equal(t, chunk1, chunk2)
						totalRead += len(chunk1)
					}

					_, err1 := chunkReader1.ReadNextChunk()
					require.Equal(t, io.EOF, err1)
					_, err2 := chunkReader2.ReadNextChunk()
					require.Equal(t, io.EOF, err2)
				}
			})
		}
	})
}

func FuzzRepMaxSfxContentDefinedChunker(f *testing.F) {
	f.Fuzz(func(t *testing.T, minSizeBytes, horizonSizeBytes int, data []byte) {
		if minSizeBytes <= 1 || horizonSizeBytes < 0 {
			return
		}

		chunker1 := cdc.NewSimpleRepMaxSfxContentDefinedChunker(
			minSizeBytes,
			horizonSizeBytes,
		)
		chunker2 := cdc.NewRepMaxSfxContentDefinedChunker(
			&cdc.NoSubstitutionBox,
			minSizeBytes,
			horizonSizeBytes,
		)

		chunkReader1 := chunker1.NewChunkReader(
			bufio.NewReader(bytes.NewBuffer(data)),
		)
		chunkReader2 := chunker2.NewChunkReader(
			bufio.NewReader(bytes.NewBuffer(data)),
		)

		for totalRead := 0; totalRead < len(data); {
			chunk1, err1 := chunkReader1.ReadNextChunk()
			require.NoError(t, err1)
			require.LessOrEqual(t, min(minSizeBytes, len(data)), len(chunk1))
			require.Greater(t, 2*minSizeBytes, len(chunk1))

			chunk2, err2 := chunkReader2.ReadNextChunk()
			require.NoError(t, err2)
			require.Equal(t, chunk1, chunk2)
			totalRead += len(chunk1)
		}

		_, err1 := chunkReader1.ReadNextChunk()
		require.Equal(t, io.EOF, err1)
		_, err2 := chunkReader2.ReadNextChunk()
		require.Equal(t, io.EOF, err2)
	})
}
