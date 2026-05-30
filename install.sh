#!/usr/bin/env bash
set -euo pipefail

REPO="KilledKenny/crossfuzz"
INSTALL_DIR="${HOME}/.local/bin"

# Detect OS
case "$(uname -s)" in
  Linux)  OS="linux"  ;;
  Darwin) OS="darwin" ;;
  *) echo "error: unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

# Detect architecture
case "$(uname -m)" in
  x86_64)        ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "error: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

# Resolve latest release tag
API="https://api.github.com/repos/${REPO}/releases/latest"
TAG=$(curl -fsSL "$API" | grep '"tag_name"' | sed 's/.*"tag_name": *"cli-v\([^"]*\)".*/\1/')
if [ -z "$TAG" ]; then
  echo "error: could not determine latest version" >&2
  exit 1
fi

BINARY="crossfuzz-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/download/cli-v${TAG}/${BINARY}"

echo "Installing crossfuzz v${TAG} (${OS}/${ARCH}) to ${INSTALL_DIR}..."

mkdir -p "$INSTALL_DIR"
curl -fsSL "$URL" -o "${INSTALL_DIR}/crossfuzz"
chmod +x "${INSTALL_DIR}/crossfuzz"

echo "Done. crossfuzz v${TAG} installed to ${INSTALL_DIR}/crossfuzz"

# Add install dir to PATH in ~/.bashrc if not already present
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    echo "warning: ${INSTALL_DIR} is not in your PATH — add it by running:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    ;;
esac
