#!/bin/sh

set -eu -o pipefail

xargs -P $(sysctl -n hw.ncpu) -n 1 ./generate_single_listing.sh < kernel_versions
