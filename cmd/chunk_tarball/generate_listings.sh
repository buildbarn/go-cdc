#!/bin/sh

set -eu -o pipefail

(cd ~/projects/linux && git tag | grep '^v6') | xargs -P 8 -n 1 ./generate_single_listing.sh
