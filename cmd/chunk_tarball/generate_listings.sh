#!/bin/sh
#
set -eu -o pipefail

for tag in $(cd ~/projects/linux && git tag | grep '^v6'); do
  echo $tag
  (cd ~/projects/linux && git archive $tag) | go run . > chunks-$tag.txt
done
