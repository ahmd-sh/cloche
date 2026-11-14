#!/bin/sh
set -eu

version=$1
mkdir -p "$2"
out=$(cd "$2" && pwd)
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

cd "$root"
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
	os=${target%/*}
	arch=${target#*/}
	name=cloche-$os-$arch
	exe=cloche
	[ "$os" = windows ] && exe=cloche.exe
	mkdir "$work/$name"
	CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$work/$name/$exe" ./cmd/cloche
	cp LICENSE README.md "$work/$name/"
	if [ "$os" = windows ]; then
		(cd "$work/$name" && zip -q -X "$out/$name.zip" "$exe" LICENSE README.md)
	else
		tar -C "$work/$name" -czf "$out/$name.tar.gz" "$exe" LICENSE README.md
	fi
done
