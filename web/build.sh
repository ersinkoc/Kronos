#!/usr/bin/env sh
set -eu

cd "$(dirname "$0")"

if command -v pnpm >/dev/null 2>&1; then
	PNPM=pnpm
elif command -v corepack >/dev/null 2>&1; then
	corepack enable
	PNPM="corepack pnpm"
else
	echo "pnpm or corepack is required to build the WebUI" >&2
	exit 1
fi

if [ -f pnpm-lock.yaml ]; then
	$PNPM install --frozen-lockfile
else
	$PNPM install
fi
$PNPM build

rm -rf ../internal/webui/static
mkdir -p ../internal/webui/static
cp -R dist/. ../internal/webui/static/
