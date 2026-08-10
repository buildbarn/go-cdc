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

func TestRepMaxContentDefinedChunkerNewChunkReader(t *testing.T) {
	// Test that RepMaxContentDefinedChunker behaves the same way as
	// SimpleRepMaxContentDefinedChunker.
	seed := rand.Int63()
	r1 := rand.New(rand.NewSource(seed))
	r2 := rand.New(rand.NewSource(seed))

	for horizonSizeBytes := 0; horizonSizeBytes <= 16*1024; horizonSizeBytes += 2 * 1024 {
		t.Run(fmt.Sprintf("Horizon=%d", horizonSizeBytes), func(t *testing.T) {
			chunker1 := cdc.NewSimpleRepMaxContentDefinedChunker(
				&cdc.FastContentDefinedChunkerGearTable,
				/* minSizeBytes = */ 2*1024,
				horizonSizeBytes,
			)
			chunker2 := cdc.NewRepMaxContentDefinedChunker(
				&cdc.FastContentDefinedChunkerGearTable,
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
}

func FuzzRepMaxContentDefinedChunker(f *testing.F) {
	f.Fuzz(func(t *testing.T, gearSeed []byte, minSizeBytes, horizonSizeBytes int, data []byte) {
		if minSizeBytes < 64 || horizonSizeBytes < 0 {
			return
		}

		gearTable := cdc.NewSeededGearTable(gearSeed)
		chunker1 := cdc.NewSimpleRepMaxContentDefinedChunker(
			gearTable,
			minSizeBytes,
			horizonSizeBytes,
		)
		chunker2 := cdc.NewRepMaxContentDefinedChunker(
			gearTable,
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

func BenchmarkRepMaxContentDefinedChunker(b *testing.B) {
	// Chunking parameters as used by Bazel for remote cache
	// chunking: a minimum chunk size of 256 KiB and a horizon of
	// eight times that.
	const (
		minSizeBytes     = 256 * 1024
		horizonSizeBytes = 8 * minSizeBytes
		peekSizeBytes    = 2*minSizeBytes + horizonSizeBytes
		sizeBytes        = 64 * 1024 * 1024
	)
	chunker := cdc.NewRepMaxContentDefinedChunker(
		&cdc.FastContentDefinedChunkerGearTable,
		minSizeBytes,
		horizonSizeBytes,
	)

	data := make([]byte, sizeBytes)
	rand.New(rand.NewSource(1)).Read(data)
	reader := bytes.NewReader(data)
	bufferedReader := bufio.NewReaderSize(reader, peekSizeBytes)

	b.SetBytes(sizeBytes)
	for b.Loop() {
		reader.Reset(data)
		bufferedReader.Reset(reader)
		chunkReader := chunker.NewChunkReader(bufferedReader)
		for {
			if _, err := chunkReader.ReadNextChunk(); err != nil {
				if err == io.EOF {
					break
				}
				b.Fatal(err)
			}
		}
	}
}

type offsetTrackingPeeker struct {
	cdc.Peeker
	offset int
}

func (p *offsetTrackingPeeker) Discard(n int) (int, error) {
	discarded, err := p.Peeker.Discard(n)
	p.offset += discarded
	return discarded, err
}

func TestRepMaxContentDefinedChunkerDiscardUpToGuaranteedChunk(t *testing.T) {
	chunker := cdc.NewRepMaxContentDefinedChunker(
		&cdc.FastContentDefinedChunkerGearTable,
		/* minSizeBytes = */ 2*1024,
		/* horizonSizeBytes = */ 16*1024,
	)

	for i := 0; i < 10000; i++ {
		// Create a random stream of data and discard a random
		// amount of data from the start. Progress the stream to
		// the next point that can no longer be influenced by
		// any data before it.
		seed := rand.Int63()
		peeker := offsetTrackingPeeker{Peeker: bufio.NewReaderSize(rand.New(rand.NewSource(seed)), 64*1024)}
		_, err := peeker.Discard(rand.Intn(16 * 1024))
		require.NoError(t, err)
		require.NoError(t, chunker.DiscardUpToGuaranteedChunk(&peeker))

		// Ensure that if we were to chunk the same stream of
		// data, that the selected cutting point also appears.
		// This is needed to get chunking invariability.
		chunkReader := chunker.NewChunkReader(
			bufio.NewReaderSize(rand.New(rand.NewSource(seed)), 64*1024),
		)
		for remainingBytes := peeker.offset; remainingBytes > 0; {
			chunk, err := chunkReader.ReadNextChunk()
			require.NoError(t, err)
			require.GreaterOrEqual(t, remainingBytes, len(chunk))
			remainingBytes -= len(chunk)
		}
	}
}
