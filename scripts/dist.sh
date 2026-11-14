#!/bin/sh
set -eu

version=$1
mkdir -p "$2"
out=$(cd "$2" && pwd)
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cd "$root"
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do
	os=${target%/*}
	arch=${target#*/}
	name=cloche-$os-$arch
	mkdir "$work/$name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$work/$name/cloche" ./cmd/cloche
	cp LICENSE README.md "$work/$name/"
	tar -C "$work/$name" -czf "$out/$name.tar.gz" cloche LICENSE README.md
done
