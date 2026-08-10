package cdc

import (
	"io"
)

type fastContentDefinedChunker struct {
	nonSynchronizableContentDefinedChunker

	gearTable       *GearTable
	minSizeBytes    int
	normalSizeBytes int
	maxSizeBytes    int
	maskS           uint64
	maskL           uint64
}

// NewFastContentDefinedChunker returns a content defined chunker that
// uses the FastCDC8KB algorithm as described in the paper "The Design
// of Fast Content-Defined Chunking for Data Deduplication Based Storage
// Systems".
func NewFastContentDefinedChunker(gearTable *GearTable, normalSizeBytes int) ContentDefinedChunker {
	return &fastContentDefinedChunker{
		gearTable:       gearTable,
		minSizeBytes:    normalSizeBytes / 4,
		normalSizeBytes: normalSizeBytes,
		maxSizeBytes:    normalSizeBytes * 4,
		maskS: map[int]uint64{
			512:   0x0000d90003530000,
			1024:  0x0000d90103530000,
			2048:  0x0000d90303530000,
			4096:  0x0000d90313530000,
			8192:  0x0000d90f03530000,
			16384: 0x0000d90303537000,
			32768: 0x0000d90703537000,
			65536: 0x0000d90707537000,
		}[normalSizeBytes],
		maskL: map[int]uint64{
			512:   0x0000000018035100,
			1024:  0x0000001800035300,
			2048:  0x0000019000353000,
			4096:  0x0000590003530000,
			8192:  0x0000d90003530000,
			16384: 0x0000d90103530000,
			32768: 0x0000d90303530000,
			65536: 0x0000d90313530000,
		}[normalSizeBytes],
	}
}

func (c *fastContentDefinedChunker) NewChunkReader(peeker Peeker) ChunkReader {
	return &fastChunkReader{
		contentDefinedChunker: c,
		peeker:                peeker,
	}
}

type fastChunkReader struct {
	contentDefinedChunker *fastContentDefinedChunker
	peeker                Peeker

	previousChunkSizeBytes int
}

func (r *fastChunkReader) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := r.peeker.Discard(r.previousChunkSizeBytes)
	r.previousChunkSizeBytes -= discardedSizeBytes
	if err != nil {
		return nil, err
	}

	// Gain access to the data corresponding to the next chunk(s).
	c := r.contentDefinedChunker
	d, err := r.peeker.Peek(c.maxSizeBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}

	gear := &c.gearTable.values
	if len(d) >= c.normalSizeBytes {
		// Large object. Use two different bitmasks.
		var hash uint64
		hashRegion := d[c.minSizeBytes:c.normalSizeBytes]
		i := 0
		for ; i+8 <= len(hashRegion); i += 8 {
			b := [8]byte(hashRegion[i : i+8])
			s := gear[b[0]]
			h := (hash << 1) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 1
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[1]]
			h = (hash << 2) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 2
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[2]]
			h = (hash << 3) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 3
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[3]]
			h = (hash << 4) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 4
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[4]]
			h = (hash << 5) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 5
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[5]]
			h = (hash << 6) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 6
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[6]]
			h = (hash << 7) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 7
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[7]]
			h = (hash << 8) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 8
				return d[:r.previousChunkSizeBytes], nil
			}
			hash = h
		}
		for ; i < len(hashRegion); i++ {
			hash = (hash << 1) + gear[hashRegion[i]]
			if hash&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i
				return d[:r.previousChunkSizeBytes], nil
			}
		}

		hashRegion = d[c.normalSizeBytes:]
		i = 0
		for ; i+8 <= len(hashRegion); i += 8 {
			b := [8]byte(hashRegion[i : i+8])
			s := gear[b[0]]
			h := (hash << 1) + s
			if h&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i + 1
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[1]]
			h = (hash << 2) + s
			if h&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i + 2
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[2]]
			h = (hash << 3) + s
			if h&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i + 3
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[3]]
			h = (hash << 4) + s
			if h&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i + 4
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[4]]
			h = (hash << 5) + s
			if h&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i + 5
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[5]]
			h = (hash << 6) + s
			if h&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i + 6
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[6]]
			h = (hash << 7) + s
			if h&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i + 7
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[7]]
			h = (hash << 8) + s
			if h&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i + 8
				return d[:r.previousChunkSizeBytes], nil
			}
			hash = h
		}
		for ; i < len(hashRegion); i++ {
			hash = (hash << 1) + gear[hashRegion[i]]
			if hash&c.maskL == 0 {
				r.previousChunkSizeBytes = c.normalSizeBytes + i
				return d[:r.previousChunkSizeBytes], nil
			}
		}
	} else if len(d) >= c.minSizeBytes {
		// Small object. Only use a single bitmask.
		var hash uint64
		hashRegion := d[c.minSizeBytes:]
		i := 0
		for ; i+8 <= len(hashRegion); i += 8 {
			b := [8]byte(hashRegion[i : i+8])
			s := gear[b[0]]
			h := (hash << 1) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 1
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[1]]
			h = (hash << 2) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 2
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[2]]
			h = (hash << 3) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 3
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[3]]
			h = (hash << 4) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 4
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[4]]
			h = (hash << 5) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 5
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[5]]
			h = (hash << 6) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 6
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[6]]
			h = (hash << 7) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 7
				return d[:r.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[7]]
			h = (hash << 8) + s
			if h&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i + 8
				return d[:r.previousChunkSizeBytes], nil
			}
			hash = h
		}
		for ; i < len(hashRegion); i++ {
			hash = (hash << 1) + gear[hashRegion[i]]
			if hash&c.maskS == 0 {
				r.previousChunkSizeBytes = c.minSizeBytes + i
				return d[:r.previousChunkSizeBytes], nil
			}
		}
	} else if len(d) == 0 {
		return nil, io.EOF
	}

	r.previousChunkSizeBytes = len(d)
	return d, nil
}
