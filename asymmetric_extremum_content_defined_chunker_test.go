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

func TestAsymmetricExtremumContentDefinedChunker(t *testing.T) {
	t.Run("Static", func(t *testing.T) {
		for _, testCase := range []struct {
			minSizeBytes int
			maxSizeBytes int
			data         string
			cuts         []int
		}{
			{
				minSizeBytes: 2,
				maxSizeBytes: 134,
				data:         "0001",
				cuts:         []int{3, 1},
			},
			{
				minSizeBytes: 19,
				maxSizeBytes: 41,
				data:         "00000a00000000000000000000",
				cuts:         []int{25, 1},
			},
		} {
			t.Run(fmt.Sprintf("%d/%d/%s", testCase.minSizeBytes, testCase.maxSizeBytes, testCase.data), func(t *testing.T) {
				chunker := cdc.NewAsymmetricExtremumContentDefinedChunker(
					testCase.minSizeBytes,
					testCase.maxSizeBytes,
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
		// Test that AsymmetricExtremumContentDefinedChunker behaves the
		// same way as SimpleAsymmetricExtremumContentDefinedChunker.
		seed := rand.Int63()
		r1 := rand.New(rand.NewSource(seed))
		r2 := rand.New(rand.NewSource(seed))

		chunker1 := cdc.NewSimpleAsymmetricExtremumContentDefinedChunker(
			/* minSizeBytes = */ 2*1024,
			/* maxSizeBytes = */ 16*1024,
		)
		chunker2 := cdc.NewAsymmetricExtremumContentDefinedChunker(
			/* minSizeBytes = */ 2*1024,
			/* maxSizeBytes = */ 16*1024,
		)

		for i := 0; i < 1000; i++ {
			chunkReader1 := chunker1.NewChunkReader(
				bufio.NewReaderSize(io.LimitReader(r1, 1024*1024), 64*1024),
			)
			chunkReader2 := chunker2.NewChunkReader(
				bufio.NewReaderSize(io.LimitReader(r2, 1024*1024), 64*1024),
			)

			for totalRead := 0; totalRead < 1024*1024; {
				chunk1, err1 := chunkReader1.ReadNextChunk()
				require.NoError(t, err1)
				require.GreaterOrEqual(t, 16*1024, len(chunk1))

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

func FuzzAsymmetricExtremumContentDefinedChunker(f *testing.F) {
	f.Fuzz(func(t *testing.T, minSizeBytes, maxSizeBytes int, data []byte) {
		if minSizeBytes < 1 || maxSizeBytes < minSizeBytes {
			return
		}

		chunker1 := cdc.NewSimpleAsymmetricExtremumContentDefinedChunker(
			minSizeBytes,
			maxSizeBytes,
		)
		chunker2 := cdc.NewAsymmetricExtremumContentDefinedChunker(
			minSizeBytes,
			maxSizeBytes,
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
			require.GreaterOrEqual(t, max(maxSizeBytes, 2*minSizeBytes), len(chunk1))

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
