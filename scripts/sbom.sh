#!/usr/bin/env sh
set -eu

go_cmd="${GO:-go}"
version="${VERSION:-dev}"
commit="${COMMIT:-unknown}"
build_date="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
out="${SBOM_OUT:-bin/kronos-sbom.json}"

mkdir -p "$(dirname "$out")"

tmp="$out.tmp"
{
	printf '{\n'
	printf '  "bomFormat": "kronos-go-modules",\n'
	printf '  "specVersion": "1.0",\n'
	printf '  "metadata": {\n'
	printf '    "component": {"type": "application", "name": "kronos", "version": "%s"},\n' "$version"
	printf '    "commit": "%s",\n' "$commit"
	printf '    "buildDate": "%s"\n' "$build_date"
	printf '  },\n'
	printf '  "components": [\n'
	first=1
	"$go_cmd" list -m all | while read -r path module_version; do
		[ -n "$path" ] || continue
		if [ "$first" -eq 0 ]; then
			printf ',\n'
		fi
		first=0
		if [ -n "${module_version:-}" ]; then
			printf '    {"type": "library", "name": "%s", "version": "%s"}' "$path" "$module_version"
		else
			printf '    {"type": "application", "name": "%s", "version": "%s"}' "$path" "$version"
		fi
	done
	printf '\n  ]\n'
	printf '}\n'
} >"$tmp"
mv "$tmp" "$out"
echo "$out"
