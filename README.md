# Content Defined Chunking playground

This repository provides implementations for a small number of
[Content Defined Chunking](https://en.wikipedia.org/wiki/Rolling_hash)
algorithms for the Go programming language. These implementations are
provided for testing/benchmarking purposes.

This repository was created in response to
[Bazel remote-apis PR #282](https://github.com/bazelbuild/remote-apis/pull/282),
where support for Content Defined Chunking is considered for inclusion
into the remote execution protocol that is used by tools like Bazel and
Buildbarn.

## MaxCDC: content defined chunking with lookahead

Algorithms like [Rabin fingerprinting](https://github.com/fd0/rabin-cdc)
and [FastCDC](https://www.usenix.org/conference/atc16/technical-sessions/presentation/xia)
all work on the basis that they perform a simple forward scan through
the input data. A decision to introduce a cutting point between chunks
is made when the bytes right before it are hashed. This simple design
has some limitations. For example:

- If no cutting point is found before the maximum chunk size is reached,
  the algorithm is forced to make a cut at an undesirable offset. It
  will not be able to select a "close to optimal" cutting point.

- When implemented trivially, the size of chunks is expected to follow a
  geometric distribution. This means that there is a relatively large
  spread in size between the smallest and largest chunks. For example,
  for FastCDC8KB the largest chunks can be 32 times as large as the
  smallest ones (2 KB vs 64 KB).

The MaxCDC algorithm attempts to address this by performing lookaheads.
Instead of selecting cutting points on the fly, it always scans the
input up to the maximum limit and only afterwards chooses a cutting
point that is most desirable. It considers the most desirable cutting
point to be the one for which the Gear hash has the highest value, hence
the name MaxCDC.

### Runtime performance

Implementing MaxCDC trivially (as done in
`simple_max_content_defined_chunker.go`) has the disadvantage that input
data is hashed redundantly. It may process input up to the maximum limit
and select a cutting point close to the minimum limit. Any data in
between those two limits would be hashed again during the next
iteration. To eliminate this overhead, we provide an optimized
implementation (in `max_content_defined_chunker.go`) that preserves
potential future cutting points on a stack, allowing subsequent calls to
reuse this information. Performance of this optimized implementation is
nearly identical to plain FastCDC.

### Deduplication performance

In order to validate the quality of the chunking performed by this
algorithm, we have created uncompressed tarballs of 80 different
versions of the Linux kernel (from v6.0 to v6.8, including all release
candidates). Each of these tarballs is approximately 1.4 GB in size.

When chunking all of these tarballs with FastCDC8KB, we see that each
tarball is split into about 145k chunks. When deduplicating chunks
across all 80 versions, 383,093 chunks remain that have a total size of
3,872,754,501 bytes. Chunks thus have an average size of 10,109 bytes.

We then chunked the same tarballs using MaxCDC, using a minimum size of
4,096 bytes and a maximum size of 14,785 bytes. After deduplicating,
this yielded 374,833 chunks having a total size of 3,790,013,152 bytes.
The minimum and maximum chunk size were intentionally chosen so that the
average chunk size was almost identical to that of FastCDC8KB, namely
10,111 bytes.

We therefore conclude that for this specific benchmark the MaxCDC
generated output consumes 2.14% less space than FastCDC8KB. Furthermore,
the spread in chunk size is also far better when using MaxCDC (14,785
B / 4,096 B ≈ 3.61) when compared to FastCDC8KB (64 KB / 2 KB = 32).

### Tuning recommendations

Assume you use MaxCDC to chunk two non-identical, but similar files.
Making the ratio between the minimum and maximum permitted chunk size
too small leads to bad performance, because it causes the streams of
chunks to take longer to converge after differing parts have finished
processing.

Conversely, making the ratio between the minimum and maximum permitted
chunk size too large is also suboptimal. The reason being that large
chunks have a lower probability of getting deduplicated against others.
This causes the average size of chunks stored in a deduplicating data
store to become higher than that of the chunking algorithm itself.

When chunking and deduplicating the Linux kernel source tarballs, we
observed that for that specific data set the optimal ratio between the
minimum and maximum chunk size was somewhere close to 4x. We therefore
recommend that this ratio is used as a starting point.

### Relationship to RDC FilterMax

Microsoft's [Remote Differential Compression algorithm](https://learn.microsoft.com/en-us/openspecs/windows_protocols/ms-rdc)
uses a content defined chunking algorithm named FilterMax. Just like
MaxCDC, it attempts to insert cutting points at positions where the hash
value of a rolling hash function is a local maximum. The main difference
is that this is only checked within a small region what the algorithm
names the horizon. This results in a chunk size distribution that is
geometric, similar to traditional Rabin fingerprinting implementations.

Some testing of this construct in combination with the Gear hash
function was performed, using the same methodology as described above.
Deduplicating yielded 398,967 unique chunks with a combined size of
4,031,959,354 bytes. This is 4.11% worse than FastCDC8KB and 6.38% worse
than MaxCDC. The average chunk size was 10,105 bytes, which is similar
to what was used for the previous tests.
