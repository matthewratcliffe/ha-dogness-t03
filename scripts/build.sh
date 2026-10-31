#!/usr/bin/env bash
# Rebuild the prebuilt go2rtc binaries in dogness_t03/bin/ from source.
#
# Clones upstream go2rtc at the pinned commit, applies dogness_t03/overlay/,
# and cross-compiles static (CGO-disabled) binaries for each supported arch.
# Requires: git, go (>= the version in go2rtc's go.mod).
set -euo pipefail

GO2RTC_REF="7c05066c90ab6ad1fccb506d5f0408aa31ebe60f"   # upstream go2rtc v1.9.15
HERE="$(cd "$(dirname "$0")/.." && pwd)"
OVERLAY="${HERE}/dogness_t03/overlay"
OUT="${HERE}/dogness_t03/bin"
WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

echo ">> cloning go2rtc @ ${GO2RTC_REF}"
git clone --quiet https://github.com/AlexxIT/go2rtc "${WORK}/src"
git -C "${WORK}/src" checkout --quiet "${GO2RTC_REF}"

echo ">> applying overlay"
cp -r "${OVERLAY}/." "${WORK}/src/"

mkdir -p "${OUT}"
cd "${WORK}/src"

build() { # <goos> <goarch> <goarm> <outfile>
  echo ">> building $4"
  CGO_ENABLED=0 GOOS="$1" GOARCH="$2" GOARM="$3" \
    go build -trimpath -ldflags "-s -w" -o "${OUT}/$4" .
}

build linux arm64 "" go2rtc-aarch64
build linux arm   7  go2rtc-armv7
build linux amd64 "" go2rtc-amd64

echo ">> done. binaries in ${OUT}"
