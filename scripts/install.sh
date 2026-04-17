#!/usr/bin/env bash
# Install, update, or uninstall go-apply from GitHub releases.
# Usage:
#   Install/update: curl -sSfL https://raw.githubusercontent.com/thedandano/go-apply/main/scripts/install.sh | bash
#   Uninstall:      curl -sSfL .../install.sh | bash -s -- --uninstall
#   Uninstall+purge: curl -sSfL .../install.sh | bash -s -- --uninstall --purge
# Options (env vars):
#   VERSION      — specific version to install (default: latest)
#   INSTALL_DIR  — where to place the binary (default: ~/.local/bin)
set -euo pipefail

REPO="thedandano/go-apply"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# --- Uninstall mode ---
if [ "${1:-}" = "--uninstall" ]; then
  BINARY="${INSTALL_DIR}/go-apply"

  # Deregister MCP server from all known agents before removing binary.
  # Failures are silenced — if an agent isn't configured, that's fine.
  if [ -f "$BINARY" ]; then
    "$BINARY" setup mcp --agent all --remove 2>/dev/null || true
  fi

  if [ -f "$BINARY" ]; then
    rm -f "$BINARY"
    echo "Removed $BINARY"
  else
    echo "go-apply not found at $BINARY"
  fi

  # Optionally remove config and data directories
  CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/go-apply"
  DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/go-apply"
  STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/go-apply"

  for dir in "$CONFIG_DIR" "$DATA_DIR" "$STATE_DIR"; do
    if [ -d "$dir" ]; then
      # In piped mode (curl | bash), stdin is the script itself, so
      # default to keeping data unless --purge is also passed.
      if [ "${2:-}" = "--purge" ]; then
        rm -rf "$dir"
        echo "Removed $dir"
      else
        echo "Kept $dir (pass --purge to also remove config, data, and logs)"
      fi
    fi
  done

  echo "go-apply uninstalled."
  exit 0
fi

# --- Install/update mode ---

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)  OS="linux" ;;
  Darwin) OS="darwin" ;;
  *)      echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Resolve version
if [ -z "${VERSION:-}" ]; then
  VERSION="$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v([^"]+)".*/\1/')"
  if [ -z "$VERSION" ]; then
    echo "Failed to determine latest version" >&2; exit 1
  fi
fi
# Strip leading 'v' if present
VERSION="${VERSION#v}"

ARCHIVE="go-apply_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/v${VERSION}"

# Create temp directory and clean up on exit
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Installing go-apply v${VERSION} (${OS}/${ARCH})..."

# Download archive and checksums
curl -sSfL -o "${TMP_DIR}/${ARCHIVE}" "${BASE_URL}/${ARCHIVE}"
curl -sSfL -o "${TMP_DIR}/checksums.txt" "${BASE_URL}/checksums.txt"

# Verify checksum
cd "$TMP_DIR"
# Prefer shasum on macOS (native); fall back to GNU sha256sum on Linux.
if command -v shasum >/dev/null 2>&1; then
  grep "${ARCHIVE}" checksums.txt | shasum -a 256 -c --quiet
elif command -v sha256sum >/dev/null 2>&1; then
  grep "${ARCHIVE}" checksums.txt | sha256sum -c --quiet
else
  echo "Warning: no sha256sum or shasum found, skipping checksum verification" >&2
fi

# Extract binary
tar xzf "${ARCHIVE}" go-apply

# Install
mkdir -p "${INSTALL_DIR}"
mv go-apply "${INSTALL_DIR}/go-apply"
chmod +x "${INSTALL_DIR}/go-apply"

echo "Installed go-apply v${VERSION} to ${INSTALL_DIR}/go-apply"

# Check PATH
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) echo "Note: ${INSTALL_DIR} is not in your PATH. Add it with:"
     echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
     ;;
esac
