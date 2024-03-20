package cdc

// ContentDefinedChunker can be used to decompose a large file into
// smaller chunks. Cutting points are determined by inspecting the
// binary content of the file.
type ContentDefinedChunker interface {
	ReadNextChunk() ([]byte, error)
}
