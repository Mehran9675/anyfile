#!/usr/bin/env bash
# Writes the release tag into giode.toml: app.version takes a plain
# x.y.z (v1.2.3-beta becomes 1.2.3, as Windows and macOS require), and
# the ldflags stamp the full tag into main.version for the app to show.
set -euo pipefail
ver="${TAG#v}"
ver="${ver%%-*}"
sed -i.bak -e "s/^version = .*/version = \"$ver\"/" \
  -e "s/^ldflags = .*/ldflags = \"-s -w -X main.version=$TAG\"/" giode.toml
rm -f giode.toml.bak
grep -E '^(version|ldflags) =' giode.toml
