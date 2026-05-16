#!/bin/bash
# Vassago Daemon installer — build from source and install to ~/bin
# Usage: curl -fsSL https://raw.githubusercontent.com/aj-nt/vassago/main/install.sh | bash

set -euo pipefail

REPO="https://github.com/aj-nt/vassago.git"
BINARY="vassago"
INSTALL_DIR="$HOME/bin"
TMPDIR_BASE="$(mktemp -d)"

cleanup() { rm -rf "$TMPDIR_BASE"; }
trap cleanup EXIT

echo "╭─────────────────────────────────────────────╮"
echo "│  Vassago Daemon Installer                │"
echo "╰─────────────────────────────────────────────╯"
echo ""

# Check prerequisites
if ! command -v go &>/dev/null; then
    echo "✗ Go is required but not installed."
    echo "  Install it: https://go.dev/dl/"
    exit 1
fi

GO_VERSION=$(go version | { read _ _ v _; echo "${v#go}"; })
echo "✓ Go $GO_VERSION found"

# Check minimum Go version (1.21)
MAJOR=$(echo "$GO_VERSION" | cut -d. -f1)
MINOR=$(echo "$GO_VERSION" | cut -d. -f2)
if [ "$MAJOR" -lt 1 ] || { [ "$MAJOR" -eq 1 ] && [ "$MINOR" -lt 21 ]; }; then
    echo "✗ Go 1.21+ required, found $GO_VERSION"
    exit 1
fi

if ! command -v make &>/dev/null; then
    echo "✗ make is required but not installed."
    exit 1
fi
echo "✓ make found"

# Clone
echo ""
echo "Cloning Vassago..."
git clone --depth 1 "$REPO" "$TMPDIR_BASE/vassago"
cd "$TMPDIR_BASE/vassago"

# Build
echo "Building..."
make build

# Install
mkdir -p "$INSTALL_DIR"
cp bin/"$BINARY" "$INSTALL_DIR/"
cp bin/vassago-daemon "$INSTALL_DIR/"
cp bin/vassago-cli "$INSTALL_DIR/"
cp bin/vassago-import "$INSTALL_DIR/"
echo "✓ Installed vassago, vassago-daemon, vassago-cli, vassago-import to $INSTALL_DIR/"

# macOS: codesign binaries (macOS 15+ kills unsigned binaries with SIGKILL)
if [[ "$(uname -s)" == "Darwin" ]]; then
    if command -v codesign &>/dev/null; then
        codesign --force --sign - "$INSTALL_DIR/$BINARY" "$INSTALL_DIR/vassago-daemon" "$INSTALL_DIR/vassago-cli" "$INSTALL_DIR/vassago-import" 2>/dev/null && echo "✓ Codesigned binaries for macOS" || echo "⚠ Codesigning failed — binaries may be killed by macOS"
    fi
fi

# Verify
if command -v "$INSTALL_DIR/$BINARY" &>/dev/null; then
    VERSION=$("$INSTALL_DIR/$BINARY" version 2>/dev/null | head -1 || echo "unknown")
    echo "✓ $VERSION"
fi

# PATH hint
echo ""
if [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
    echo "⚠ Add $INSTALL_DIR to your PATH:"
    echo "  echo 'export PATH="$INSTALL_DIR:\$PATH"' >> ~/.bashrc"
    echo "  source ~/.bashrc"
    echo ""
fi

echo "Next steps:"
echo "  1. vassago init       # interactive setup"
echo "  2. vassago start      # start the daemon"
echo "  3. Connect an AI agent to the daemon via gRPC"
echo ""
echo "╭─────────────────────────────────────────────╮"
echo "│  Installation complete!                     │"
echo "╰─────────────────────────────────────────────╯"
