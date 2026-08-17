# Content Defined Chunking playground

This repository provides reference implementations of the
RepMaxCDC "Repeated Maximum" [Content-Defined
Chunking](https://en.wikipedia.org/wiki/Rolling_hash) function, which is
written in the Go programming language. RepMaxCDC is
[one of the standard CDC functions of Bazel's remote execution protocol](https://github.com/bazelbuild/remote-apis/pull/282).
An implementation written in Java
[is part of Bazel](https://github.com/bazelbuild/bazel/pull/30131).

RepMaxCDC provides:

- **Tight chunk size bounds:** Most CDC functions generate chunks
  whose minimum and maximum size are still a factor of 16 or 32 apart.
  RepMaxCDC is capable of generating chunks with sizes in range
  $[n, 2n)$, while offering excellent deduplication rates.

- **Excellent parallelism:** RepMaxCDC allows performing targeted
  searches for cutting points. This makes it possible to partition a
  large file into roughly equally sized pieces. These can be chunked in
  parallel.

- **Size-based checking:** With chunks always falling in range $[n, 2n)$,
  it is trivial to check whether a file can be split into multiple
  chunks, purely looking at its size. This property, which other CDC
  functions often lack, was needed to add support for chunking to
  Bazel's existing remote execution protocol in a backward compatible
  way.

Two implementations of RepMaxCDC are included:

- [`simple_rep_max_content_defined_chunker.go`](/simple_rep_max_content_defined_chunker.go):
  A very simple, but inefficient implementation that hashes input data
  repeatedly.

- [`rep_max_content_defined_chunker.go`](/rep_max_content_defined_chunker.go):
  An optimized implementation that eliminates redundant hashing by
  storing state in lists that are preserved across calls.

Tests are used to validate that both implementations yield the same
results.

This repository also contains a copy of a paper titled
["Content-Defined Chunking with tight chunk size bounds"](/papers/cdc.pdf),
which provides a formal description and analysis of RepMaxCDC. It also
describes some simpler algorithms on which RepMaxCDC is based: MaxCDC
("Maximum") and RecMaxCDC ("Recursive Maximum"). This paper can be
referenced as follows:

```bibtex
@misc{repmaxcdc,
      title = {Content-Defined Chunking with tight chunk size bounds},
      author = {Ed Schouten},
      year = {2026},
      month = aug,
      url = {https://github.com/buildbarn/go-cdc/blob/main/papers/cdc.pdf},
}
```
