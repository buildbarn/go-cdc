#!/usr/bin/env python3

from collections import Counter
import sys

unique_chunks = set()
chunk_sizes_before_deduplication = Counter()
for line in sys.stdin:
    hash, size_bytes = line.strip().split("-")
    size_bytes = int(size_bytes)
    unique_chunks.add((hash, size_bytes))
    chunk_sizes_before_deduplication[size_bytes] += 1
print("Unique chunks: ", len(unique_chunks))

total_chunk_size = 0
chunk_sizes_after_deduplication = Counter()
for _, size_bytes in unique_chunks:
    total_chunk_size += size_bytes
    chunk_sizes_after_deduplication[size_bytes] += 1
print("Total chunk size: ", total_chunk_size)
print("Average chunk size: ", total_chunk_size / len(unique_chunks))


samples_before_deduplication = sum(
    e[1] for e in chunk_sizes_before_deduplication.items()
)
samples_after_deduplication = sum(e[1] for e in chunk_sizes_after_deduplication.items())
cumulative_count_before_deduplication = 0
cumulative_count_after_deduplication = 0
with open("cumulative-sizes.csv", "w") as f:
    for size, count_before_deduplication in sorted(
        chunk_sizes_before_deduplication.items()
    ):
        count_after_deduplication = chunk_sizes_after_deduplication[size]
        cumulative_count_before_deduplication += count_before_deduplication
        cumulative_count_after_deduplication += count_after_deduplication
        print(
            "%d,%f,%f"
            % (
                size,
                cumulative_count_before_deduplication / samples_before_deduplication,
                cumulative_count_after_deduplication / samples_after_deduplication,
            ),
            file=f,
        )
assert cumulative_count_before_deduplication == samples_before_deduplication
assert cumulative_count_after_deduplication == samples_after_deduplication
