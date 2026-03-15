#!/bin/sh
# Install script for cc-setup
# Usage: curl -fsSL https://raw.githubusercontent.com/cc-deck/cc-setup/main/install.sh | sh
#
# Environment variables:
#   INSTALL_DIR  - Installation directory (default: ~/.local/bin)
#   VERSION      - Specific version to install (default: latest)

set -e

REPO="cc-deck/cc-setup"
BINARY="cc-setup"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS
detect_os() {
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        darwin) echo "darwin" ;;
        linux)  echo "linux" ;;
        *)
            echo "Error: unsupported OS: $os" >&2
            exit 1
            ;;
    esac
}

# Detect architecture
detect_arch() {
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)
            echo "Error: unsupported architecture: $arch" >&2
            exit 1
            ;;
    esac
}

# Get latest version tag from GitHub API (no jq dependency)
get_latest_version() {
    url="https://api.github.com/repos/${REPO}/releases/latest"
    tag=$(curl -fsSL "$url" | grep '"tag_name"' | sed -E 's/.*"tag_name":\s*"([^"]+)".*/\1/')
    if [ -z "$tag" ]; then
        echo "Error: could not determine latest version" >&2
        exit 1
    fi
    echo "$tag"
}

# Verify SHA256 checksum
verify_checksum() {
    archive_file="$1"
    checksums_file="$2"

    expected=$(grep "$(basename "$archive_file")" "$checksums_file" | awk '{print $1}')
    if [ -z "$expected" ]; then
        echo "Error: no checksum found for $(basename "$archive_file")" >&2
        return 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$archive_file" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$archive_file" | awk '{print $1}')
    else
        echo "Warning: no sha256sum or shasum found, skipping checksum verification" >&2
        return 0
    fi

    if [ "$actual" != "$expected" ]; then
        echo "Error: checksum mismatch" >&2
        echo "  expected: $expected" >&2
        echo "  actual:   $actual" >&2
        return 1
    fi
}

main() {
    os=$(detect_os)
    arch=$(detect_arch)

    if [ -n "$VERSION" ]; then
        version="$VERSION"
    else
        echo "Fetching latest version..."
        version=$(get_latest_version)
    fi

    # Strip leading 'v' for the archive name
    version_num="${version#v}"

    archive="${BINARY}-${version_num}-${os}-${arch}.tar.gz"
    base_url="https://github.com/${REPO}/releases/download/${version}"

    tmpdir=$(mktemp -d)
    trap 'rm -rf "$tmpdir"' EXIT

    echo "Downloading ${BINARY} ${version} for ${os}/${arch}..."
    curl -fsSL -o "${tmpdir}/${archive}" "${base_url}/${archive}"
    curl -fsSL -o "${tmpdir}/checksums.txt" "${base_url}/checksums.txt"

    echo "Verifying checksum..."
    verify_checksum "${tmpdir}/${archive}" "${tmpdir}/checksums.txt"

    echo "Extracting..."
    tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}"

    mkdir -p "$INSTALL_DIR"
    install -m 755 "${tmpdir}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

    echo ""
    echo "Installed ${BINARY} ${version} to ${INSTALL_DIR}/${BINARY}"

    # Check if INSTALL_DIR is in PATH
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *)
            echo ""
            echo "Note: ${INSTALL_DIR} is not in your PATH."
            echo "Add it with:"
            echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
            ;;
    esac
}

main
