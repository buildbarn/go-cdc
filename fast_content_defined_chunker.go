package cdc

import (
	"io"
)

type fastContentDefinedChunker struct {
	r         Peeker
	gearTable *GearTable

	previousChunkSizeBytes int
}

// NewFastContentDefinedChunker returns a content defined chunker that
// uses the FastCDC8KB algorithm as described in the paper "The Design
// of Fast Content-Defined Chunking for Data Deduplication Based Storage
// Systems".
func NewFastContentDefinedChunker(r Peeker, gearTable *GearTable) ContentDefinedChunker {
	return &fastContentDefinedChunker{
		r:         r,
		gearTable: gearTable,
	}
}

func (c *fastContentDefinedChunker) ReadNextChunk() ([]byte, error) {
	// Discard data that was handed out by the previous call.
	discardedSizeBytes, err := c.r.Discard(c.previousChunkSizeBytes)
	c.previousChunkSizeBytes -= discardedSizeBytes
	if err != nil {
		return nil, err
	}

	const (
		minSizeBytes    = 2 * 1024
		normalSizeBytes = 8 * 1024
		maxSizeBytes    = 64 * 1024
		maskS           = 0x0000d9f003530000
		maskL           = 0x0000d90003530000
	)

	// Gain access to the data corresponding to the next chunk(s).
	d, err := c.r.Peek(maxSizeBytes)
	if err != nil && err != io.EOF {
		return nil, err
	}

	gear := &c.gearTable.values
	if len(d) >= normalSizeBytes {
		// Large object. Use two different bitmasks.
		var hash uint64
		hashRegion := d[minSizeBytes:normalSizeBytes]
		i := 0
		for ; i+8 <= len(hashRegion); i += 8 {
			b := [8]byte(hashRegion[i : i+8])
			s := gear[b[0]]
			h := (hash << 1) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 1
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[1]]
			h = (hash << 2) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 2
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[2]]
			h = (hash << 3) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 3
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[3]]
			h = (hash << 4) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 4
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[4]]
			h = (hash << 5) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 5
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[5]]
			h = (hash << 6) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 6
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[6]]
			h = (hash << 7) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 7
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[7]]
			h = (hash << 8) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 8
				return d[:c.previousChunkSizeBytes], nil
			}
			hash = h
		}
		for ; i < len(hashRegion); i++ {
			hash = (hash << 1) + gear[hashRegion[i]]
			if hash&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i
				return d[:c.previousChunkSizeBytes], nil
			}
		}

		hashRegion = d[normalSizeBytes:]
		i = 0
		for ; i+8 <= len(hashRegion); i += 8 {
			b := [8]byte(hashRegion[i : i+8])
			s := gear[b[0]]
			h := (hash << 1) + s
			if h&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i + 1
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[1]]
			h = (hash << 2) + s
			if h&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i + 2
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[2]]
			h = (hash << 3) + s
			if h&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i + 3
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[3]]
			h = (hash << 4) + s
			if h&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i + 4
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[4]]
			h = (hash << 5) + s
			if h&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i + 5
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[5]]
			h = (hash << 6) + s
			if h&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i + 6
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[6]]
			h = (hash << 7) + s
			if h&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i + 7
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[7]]
			h = (hash << 8) + s
			if h&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i + 8
				return d[:c.previousChunkSizeBytes], nil
			}
			hash = h
		}
		for ; i < len(hashRegion); i++ {
			hash = (hash << 1) + gear[hashRegion[i]]
			if hash&maskL == 0 {
				c.previousChunkSizeBytes = normalSizeBytes + i
				return d[:c.previousChunkSizeBytes], nil
			}
		}
	} else if len(d) >= minSizeBytes {
		// Small object. Only use a single bitmask.
		var hash uint64
		hashRegion := d[minSizeBytes:]
		i := 0
		for ; i+8 <= len(hashRegion); i += 8 {
			b := [8]byte(hashRegion[i : i+8])
			s := gear[b[0]]
			h := (hash << 1) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 1
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[1]]
			h = (hash << 2) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 2
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[2]]
			h = (hash << 3) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 3
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[3]]
			h = (hash << 4) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 4
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[4]]
			h = (hash << 5) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 5
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[5]]
			h = (hash << 6) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 6
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[6]]
			h = (hash << 7) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 7
				return d[:c.previousChunkSizeBytes], nil
			}
			s = (s << 1) + gear[b[7]]
			h = (hash << 8) + s
			if h&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i + 8
				return d[:c.previousChunkSizeBytes], nil
			}
			hash = h
		}
		for ; i < len(hashRegion); i++ {
			hash = (hash << 1) + gear[hashRegion[i]]
			if hash&maskS == 0 {
				c.previousChunkSizeBytes = minSizeBytes + i
				return d[:c.previousChunkSizeBytes], nil
			}
		}
	} else if len(d) == 0 {
		return nil, io.EOF
	}

	c.previousChunkSizeBytes = len(d)
	return d, nil
}
