#!/usr/bin/env bash
# Writes the release version into services/version.go before a build.
# Accepts "0.1.0" or "v0.1.0"; the UI shows the v-prefixed form.
set -euo pipefail

version="${1#v}"
if [ -z "$version" ]; then
    echo "usage: set-version.sh <version>" >&2
    exit 1
fi

cd "$(dirname "$0")/../.."
sed -i.bak -E "s#^var Version = \".*\"#var Version = \"v${version}\"#" services/version.go
rm -f services/version.go.bak
grep '^var Version' services/version.go
