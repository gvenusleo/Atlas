#!/usr/bin/env bash
set -euo pipefail

REPO="gvenusleo/atlas"
INSTALL_DIR="${ATLAS_INSTALL_DIR:-$HOME/.local/bin}"

info() { printf '\033[1;34m%s\033[0m\n' "$*"; }
err()  { printf '\033[1;31merror: %s\033[0m\n' "$*" >&2; exit 1; }

# Parse arguments
version=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -v|--version)
      version="${2:-}"
      shift 2
      ;;
    -h|--help)
      cat <<EOF
Atlas Installer

Usage: install.sh [options]

Options:
    -h, --help              Display this help message
    -v, --version <version> Install a specific version (e.g., 0.11.0)

Examples:
    curl -fsSL https://github.com/$REPO/releases/latest/download/install.sh | bash
    curl -fsSL https://github.com/$REPO/releases/latest/download/install.sh | bash -s -- --version 0.11.0
EOF
      exit 0
      ;;
    *)
      err "Unknown option: $1"
      ;;
  esac
done

# Detect OS
OS="$(uname -s)"
case "$OS" in
  Linux)  os="linux" ;;
  Darwin) os="darwin" ;;
  *)      err "Unsupported OS: $OS (try building from source: https://github.com/$REPO)" ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)        arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)             err "Unsupported architecture: $ARCH" ;;
esac

# Resolve version
if [[ -z "$version" ]]; then
  info "Fetching latest version..."
  version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | head -1 | cut -d'"' -f4)
  [[ -n "$version" ]] || err "Failed to determine latest version"
  version="${version#v}"
fi

artifact="atlas-${os}-${arch}-v${version}.tar.gz"
url="https://github.com/$REPO/releases/download/v${version}/${artifact}"

info "Installing Atlas v${version} (${os}/${arch})"

# Download
tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

info "Downloading $artifact..."
curl -fsSL "$url" -o "$tmpdir/$artifact" || err "Download failed: $url"

# Extract
mkdir -p "$INSTALL_DIR"
tar xzf "$tmpdir/$artifact" -C "$tmpdir"
mv "$tmpdir/atlas" "$INSTALL_DIR/atlas"
chmod +x "$INSTALL_DIR/atlas"

# PATH check
case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    info "Atlas installed to $INSTALL_DIR/atlas"
    ;;
  *)
    info "Atlas installed to $INSTALL_DIR/atlas"
    info ""
    info "Add Atlas to your PATH by adding this line to your shell profile:"
    info ""
    if [[ -n "${ZSH_VERSION:-}" ]] || [[ "$SHELL" == */zsh ]]; then
      info "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc"
    elif [[ -n "${BASH_VERSION:-}" ]] || [[ "$SHELL" == */bash ]]; then
      info "  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.bashrc"
    else
      info "  export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
    info ""
    info "Then restart your shell or run: source ~/.zshrc (or ~/.bashrc)"
    ;;
esac

# Verify
if "$INSTALL_DIR/atlas" version >/dev/null 2>&1; then
  info "Verification: $("$INSTALL_DIR/atlas" version)"
else
  info "Installation complete. Run 'atlas version' to verify."
fi
