#!/bin/sh
set -eu

repo=https://github.com/ahmd-sh/cloche
base=https://github.com/ahmd-sh/cloche/releases/latest/download

say() {
	printf '%s\n' "$@"
}

fail() {
	printf 'cloche install: %s\n' "$*" >&2
	exit 1
}

fetch() {
	if command -v curl >/dev/null 2>&1; then
		curl --proto '=https' --tlsv1.2 -fsSL -o "$2" "$1"
	elif command -v wget >/dev/null 2>&1; then
		wget --https-only -q -O "$2" "$1"
	else
		fail "this needs curl or wget"
	fi
}

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d ' ' -f 1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d ' ' -f 1
	else
		fail "this needs sha256sum or shasum"
	fi
}

add_to_path() {
	case ":$PATH:" in
	*":$1:"*) return ;;
	esac
	case $(basename "${SHELL:-sh}") in
	zsh)
		rc=${ZDOTDIR:-$HOME}/.zshrc
		line="export PATH=\"$1:\$PATH\""
		;;
	bash)
		rc=$HOME/.bashrc
		[ "$os" = darwin ] && rc=$HOME/.bash_profile
		line="export PATH=\"$1:\$PATH\""
		;;
	fish)
		rc=$HOME/.config/fish/config.fish
		line="fish_add_path $1"
		;;
	*)
		say "Add $1 to your PATH."
		PATH=$1:$PATH
		return
		;;
	esac
	if ! grep -qsxF "$line" "$rc"; then
		mkdir -p "$(dirname "$rc")"
		printf '\n%s\n' "$line" >>"$rc"
		say "Added $1 to your PATH in $rc. New terminals will have it."
	fi
	PATH=$1:$PATH
}

main() {
	if [ "$(id -u)" = 0 ] && [ -n "${SUDO_USER:-}" ]; then
		fail "run this without sudo. Cloche installs for your user."
	fi
	case $(uname -s) in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) fail "this is for macOS and Linux. For Windows, see $repo" ;;
	esac
	case $(uname -m) in
	x86_64 | amd64) arch=amd64 ;;
	arm64 | aarch64) arch=arm64 ;;
	*) fail "there's no build for $(uname -m). Build it from source: go install github.com/ahmd-sh/cloche/cmd/cloche@latest" ;;
	esac
	if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null)" = 1 ]; then
		arch=arm64
	fi

	dir=${CLOCHE_INSTALL_DIR:-$HOME/.local/bin}
	name=cloche-$os-$arch.tar.gz
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT

	say "Downloading $name"
	fetch "$base/$name" "$tmp/$name"
	fetch "$base/checksums.txt" "$tmp/checksums.txt"
	want=$(awk -v n="$name" '$2 == n { print $1 }' "$tmp/checksums.txt")
	[ -n "$want" ] || fail "checksums.txt doesn't list $name"
	[ "$(sha256 "$tmp/$name")" = "$want" ] || fail "$name doesn't match its checksum"

	tar -xzf "$tmp/$name" -C "$tmp" cloche
	mkdir -p "$dir"
	cp "$tmp/cloche" "$dir/.cloche.new"
	chmod 755 "$dir/.cloche.new"
	mv -f "$dir/.cloche.new" "$dir/cloche"
	say "Installed $("$dir/cloche" version) in $dir"
	add_to_path "$dir"
	say ""
	"$dir/cloche" setup || :
}

main "$@"
