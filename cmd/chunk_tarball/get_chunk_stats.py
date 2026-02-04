#!/usr/bin/env python3

import sys

unique_chunks = set(line.strip() for line in sys.stdin)
print("Unique chunks: ", len(unique_chunks))

total_chunk_size = 0
for chunk in unique_chunks:
    total_chunk_size += int(chunk.split("-")[1])
print("Total chunk size: ", total_chunk_size)
print("Average chunk size: ", total_chunk_size / len(unique_chunks))
