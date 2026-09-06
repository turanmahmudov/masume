#!/bin/sh
set -eu

REPO="turanmahmudov/masume"
INSTALL_DIR="${MASUME_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	echo "install: $1" >&2
	exit 1
}

resolve_platform() {
	os=$(uname -s)
	case "$os" in
	Linux) os=linux ;;
	Darwin) os=darwin ;;
	*) fail "masume has no build for $os" ;;
	esac

	arch=$(uname -m)
	case "$arch" in
	x86_64 | amd64) arch=amd64 ;;
	aarch64 | arm64) arch=arm64 ;;
	*) fail "masume has no build for $arch" ;;
	esac

	echo "${os}_${arch}"
}

# The releases/latest redirect carries the tag, and it has no API rate limit.
resolve_latest_tag() {
	url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
		"https://github.com/$REPO/releases/latest") ||
		fail "the latest release cannot be read"
	tag=${url##*/}
	[ -n "$tag" ] && [ "$tag" != "latest" ] || fail "the latest release has no tag"
	echo "$tag"
}

calculate_checksum() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		fail "sha256sum or shasum is required to verify the download"
	fi
}

verify_checksum() {
	expected=$(grep " $2\$" "$3" | cut -d' ' -f1)
	[ -n "$expected" ] || fail "$2 is not listed in checksums.txt"
	actual=$(calculate_checksum "$1")
	[ "$expected" = "$actual" ] ||
		fail "$2 does not match its checksum; expected $expected, got $actual"
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

platform=$(resolve_platform)
tag=${MASUME_VERSION:-$(resolve_latest_tag)}
version=${tag#v}
archive="masume_${version}_${platform}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "downloading masume $tag for $platform"
curl -fsSL "$base/$archive" -o "$tmp/$archive" ||
	fail "$archive cannot be downloaded from $base"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" ||
	fail "checksums.txt cannot be downloaded from $base"

verify_checksum "$tmp/$archive" "$archive" "$tmp/checksums.txt"
tar -xzf "$tmp/$archive" -C "$tmp" masume || fail "$archive holds no masume binary"

mkdir -p "$INSTALL_DIR" || fail "$INSTALL_DIR cannot be created"
cp "$tmp/masume" "$INSTALL_DIR/masume.new" ||
	fail "$INSTALL_DIR is not writable; set MASUME_INSTALL_DIR to another directory"
chmod 755 "$INSTALL_DIR/masume.new"
mv "$INSTALL_DIR/masume.new" "$INSTALL_DIR/masume"

echo "installed masume $tag to $INSTALL_DIR/masume"
case ":$PATH:" in
*":$INSTALL_DIR:"*) ;;
*) echo "add $INSTALL_DIR to PATH to run masume" ;;
esac
