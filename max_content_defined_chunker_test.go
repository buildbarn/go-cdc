package cdc_test

import (
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
			io.LimitReader(r1, 1024*1024),
			/* peekSizeBytes = */ 64*1024,
			/* minSizeBytes = */ 2*1024,
			/* maxSizeBytes = */ 16*1024,
		)
		chunker2 := cdc.NewMaxContentDefinedChunker(
			io.LimitReader(r2, 1024*1024),
			/* peekSizeBytes = */ 64*1024,
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
