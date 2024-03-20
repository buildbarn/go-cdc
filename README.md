# Content Defined Chunking playground

This repository provides implementations for a small number of
[Content Defined Chunking](https://en.wikipedia.org/wiki/Rolling_hash)
algorithms for the Go programming language. These implementations are
merely provided for testing purposes.

One implementation is provided for an algorithm that we call MaxCDC. It
is similar to FastCDC, with the main difference that it prefers placing
cutting points at positions where the rolling hash value is maximal.
Testing against source code of multiple versions of the Linux kernel
seems to indicate that this strategy is about 7% better at eliminating
redundancy than plain FastCDC.

More discussion can be found in
[Bazel remote-apis PR #282](https://github.com/bazelbuild/remote-apis/pull/282).
