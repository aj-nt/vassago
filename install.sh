#!/bin/bash
# Mnemosyne Daemon installer — build from source and install to ~/bin
# Usage: curl -fsSL https://raw.githubusercontent.com/aj-nt/mnemosyne/main/install.sh | bash

set -euo pipefail

REPO="https://github.com/aj-nt/mnemosyne.git"
BINARY="mnemosyne"
INSTALL_DIR="$HOME/bin"
TMPDIR_BASE="$(mktemp -d)"

cleanup() { rm -rf "$TMPDIR_BASE"; }
trap cleanup EXIT

echo "╭─────────────────────────────────────────────╮"
echo "│  Mnemosyne Daemon Installer                │"
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
echo "Cloning Mnemosyne..."
git clone --depth 1 "$REPO" "$TMPDIR_BASE/mnemosyne"
cd "$TMPDIR_BASE/mnemosyne"

# Build
echo "Building..."
make build

# Install
mkdir -p "$INSTALL_DIR"
cp bin/"$BINARY" "$INSTALL_DIR/"
cp bin/mnemosyne-daemon "$INSTALL_DIR/"
cp bin/mnemosyne-cli "$INSTALL_DIR/"
echo "✓ Installed mnemosyne, mnemosyne-daemon, mnemosyne-cli to $INSTALL_DIR/"

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
echo "  1. mnemosyne init       # interactive setup"
echo "  2. mnemosyne start      # start the daemon"
echo "  3. Connect an agent like mnemoagent to the daemon"
echo ""
echo "╭─────────────────────────────────────────────╮"
echo "│  Installation complete!                     │"
echo "╰─────────────────────────────────────────────╯"
