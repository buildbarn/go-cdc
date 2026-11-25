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

func TestBytesPeeker(t *testing.T) {
	t.Run("MatchesBufioReader", func(t *testing.T) {
		// Apply random sequences of operations to both a
		// BytesPeeker and a bufio.Reader whose buffer is large
		// enough to hold all data. Both should behave
		// identically.
		r := rand.New(rand.NewSource(rand.Int63()))
		for i := 0; i < 1000; i++ {
			data := make([]byte, r.Intn(1000))
			r.Read(data)
			p1 := cdc.NewBytesPeeker(data)
			p2 := bufio.NewReaderSize(bytes.NewReader(data), 2000)
			for j := 0; j < 20; j++ {
				n := r.Intn(len(data) + 100)
				if r.Intn(2) == 0 {
					d1, err1 := p1.Peek(n)
					d2, err2 := p2.Peek(n)
					require.Equal(t, d2, d1)
					require.Equal(t, err2, err1)
				} else {
					n1, err1 := p1.Discard(n)
					n2, err2 := p2.Discard(n)
					require.Equal(t, n2, n1)
					require.Equal(t, err2, err1)
				}
			}
		}
	})

	t.Run("NegativeCount", func(t *testing.T) {
		p := cdc.NewBytesPeeker([]byte("Hello"))
		_, err := p.Peek(-1)
		require.Equal(t, bufio.ErrNegativeCount, err)
		_, err = p.Discard(-1)
		require.Equal(t, bufio.ErrNegativeCount, err)
	})

	t.Run("NoAppendingIntoData", func(t *testing.T) {
		// Appending to slices returned by Peek() should not
		// overwrite data that has not been peeked at.
		data := []byte("Hello world")
		p := cdc.NewBytesPeeker(data)
		d, err := p.Peek(5)
		require.NoError(t, err)
		_ = append(d, 'X')
		require.Equal(t, []byte("Hello world"), data)
	})

	t.Run("Chunking", func(t *testing.T) {
		// Chunking data through a BytesPeeker should yield the
		// same chunks as through a bufio.Reader.
		chunker := cdc.NewRepMaxContentDefinedChunker(
			&cdc.FastContentDefinedChunkerGearTable,
			/* minSizeBytes = */ 2*1024,
			/* horizonSizeBytes = */ 16*1024,
		)
		r := rand.New(rand.NewSource(rand.Int63()))
		for i := 0; i < 100; i++ {
			data := make([]byte, r.Intn(256*1024))
			r.Read(data)
			chunkReader1 := chunker.NewChunkReader(cdc.NewBytesPeeker(data))
			chunkReader2 := chunker.NewChunkReader(
				bufio.NewReaderSize(bytes.NewReader(data), chunker.GetMaximumPeekSizeBytes()),
			)
			for {
				chunk1, err1 := chunkReader1.ReadNextChunk()
				chunk2, err2 := chunkReader2.ReadNextChunk()
				require.Equal(t, chunk2, chunk1)
				require.Equal(t, err2, err1)
				if err1 == io.EOF {
					break
				}
				require.NoError(t, err1)
			}
		}
	})
}
