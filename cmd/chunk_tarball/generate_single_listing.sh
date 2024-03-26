#!/bin/sh

set -eu -o pipefail

echo $1
(cd ~/projects/linux && git archive $1) | go run . > chunks-$1.txt
