#!/bin/sh
# Installer for mapps (github.com/rus-lan/multiApps).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/rus-lan/multiApps/main/install.sh | sh
#
# Env overrides:
#   MAPPS_VERSION      pin a release, e.g. v0.1.0 (default: latest)
#   MAPPS_INSTALL_DIR  install location (default: $HOME/.local/bin)
set -eu

REPO="rus-lan/multiApps"

log() {
	# All human-facing output goes to stderr so stdout stays clean under curl|sh.
	echo "$@" >&2
}

err() {
	log "error: $@"
	exit 1
}

detect_os() {
	uname_s="$(uname -s)"
	case "$uname_s" in
	Linux) echo "linux" ;;
	Darwin) echo "darwin" ;;
	*)
		log "unsupported OS: $uname_s. Windows users: use install.ps1 in PowerShell, or run this inside WSL."
		exit 1
		;;
	esac
}

detect_arch() {
	uname_m="$(uname -m)"
	case "$uname_m" in
	x86_64 | amd64) echo "amd64" ;;
	aarch64 | arm64) echo "arm64" ;;
	*)
		log "unsupported architecture: $uname_m"
		exit 1
		;;
	esac
}

# download <url> <dest>
download() {
	url="$1"
	dest="$2"
	if [ "$DOWNLOADER" = "curl" ]; then
		curl -fsSL "$url" -o "$dest"
	else
		wget -qO "$dest" "$url"
	fi
}

pick_downloader() {
	if command -v curl >/dev/null 2>&1; then
		echo "curl"
	elif command -v wget >/dev/null 2>&1; then
		echo "wget"
	else
		err "need curl or wget to download mapps, found neither"
	fi
}

# make_temp_file <suffix> - suffix keeps calls in the same script run from colliding
# when falling back to a PID-based name (no mktemp available).
make_temp_file() {
	suffix="$1"
	if command -v mktemp >/dev/null 2>&1; then
		mktemp
	else
		echo "${TMPDIR:-/tmp}/mapps.$$.${suffix}"
	fi
}

# verify_checksum <asset-name> <binary-file> <checksums-file>
verify_checksum() {
	asset="$1"
	bin_file="$2"
	sums_file="$3"

	if command -v sha256sum >/dev/null 2>&1; then
		hash_cmd="sha256sum"
	elif command -v shasum >/dev/null 2>&1; then
		hash_cmd="shasum -a 256"
	else
		log "WARNING: cannot verify checksum: no sha256sum/shasum found; continuing"
		return 0
	fi

	expected="$(grep " $asset\$" "$sums_file" 2>/dev/null | awk '{print $1}')"
	if [ -z "$expected" ]; then
		err "checksum for $asset not found in checksums.txt"
	fi

	actual="$($hash_cmd "$bin_file" | awk '{print $1}')"
	if [ "$expected" != "$actual" ]; then
		err "checksum mismatch for $asset: expected $expected, got $actual"
	fi
}

main() {
	os="$(detect_os)"
	arch="$(detect_arch)"
	asset="mapps_${os}_${arch}"

	VERSION="${MAPPS_VERSION:-}"
	if [ -z "$VERSION" ]; then
		base="https://github.com/${REPO}/releases/latest/download"
	else
		base="https://github.com/${REPO}/releases/download/${VERSION}"
	fi

	DOWNLOADER="$(pick_downloader)"

	bin_tmp="$(make_temp_file bin)"
	sums_tmp="$(make_temp_file sums)"
	trap 'rm -f "$bin_tmp" "$sums_tmp"' EXIT

	log "downloading $asset..."
	download "${base}/${asset}" "$bin_tmp"
	download "${base}/checksums.txt" "$sums_tmp"

	log "verifying checksum..."
	verify_checksum "$asset" "$bin_tmp" "$sums_tmp"

	INSTALL_DIR="${MAPPS_INSTALL_DIR:-$HOME/.local/bin}"
	mkdir -p "$INSTALL_DIR"
	mv "$bin_tmp" "$INSTALL_DIR/mapps"
	chmod +x "$INSTALL_DIR/mapps"

	case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		log ""
		log "note: $INSTALL_DIR is not on your PATH."
		log "Add this line to your shell rc (~/.bashrc, ~/.zshrc, or ~/.profile):"
		log ""
		log "  export PATH=\"$INSTALL_DIR:\$PATH\""
		log ""
		;;
	esac

	log ""
	log "mapps installed: $("$INSTALL_DIR/mapps" version)"
	log "run \"$INSTALL_DIR/mapps\" version to check it any time; once PATH is set, plain \"mapps version\" works too."
}

main "$@"
