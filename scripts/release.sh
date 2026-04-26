#!/usr/bin/env sh
set -eu

targets="${RELEASE_TARGETS:-linux/amd64 linux/arm64 darwin/amd64 darwin/arm64}"

for target in $targets; do
	./scripts/build.sh "$target"
done
