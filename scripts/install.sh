#!/usr/bin/env bash
# AI-Box installer — downloads pre-built binaries from GitHub Releases.
# Usage: curl -fsSL https://raw.githubusercontent.com/krukkeniels/ai-box/main/scripts/install.sh | bash
#   or:  ./install.sh [--version v1.0.0] [--dir /usr/local/bin] [--no-verify] [--help]
set -euo pipefail

# --- Configuration -----------------------------------------------------------

REPO="krukkeniels/ai-box"
BINARIES="aibox aibox-credential-helper aibox-llm-proxy aibox-git-remote-helper"

# --- Color helpers ------------------------------------------------------------

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Disable colors when not connected to a terminal.
if [ ! -t 1 ]; then
    RED="" GREEN="" YELLOW="" BLUE="" BOLD="" NC=""
fi

info()  { printf "${BLUE}==>${NC} ${BOLD}%s${NC}\n" "$*"; }
ok()    { printf "  ${GREEN}[ok]${NC} %s\n" "$*"; }
warn()  { printf "  ${YELLOW}[warn]${NC} %s\n" "$*" >&2; }
fail()  { printf "  ${RED}[fail]${NC} %s\n" "$*" >&2; cleanup; exit 1; }

# --- Argument parsing ---------------------------------------------------------

VERSION=""
INSTALL_DIR=""
NO_VERIFY=false

usage() {
    cat <<EOF
AI-Box Installer

Usage: install.sh [OPTIONS]

Options:
  --version <tag>    Install a specific version (default: latest)
  --dir <path>       Install directory (default: auto-detect)
  --no-verify        Skip SHA256 checksum verification
  -h, --help         Show this help message

Install directory priority:
  1. --dir <path> if specified
  2. /usr/local/bin if writable or sudo available
  3. ~/.local/bin as fallback

Examples:
  curl -fsSL https://raw.githubusercontent.com/${REPO}/main/scripts/install.sh | bash
  ./install.sh --version v1.0.0
  ./install.sh --dir ~/.local/bin
EOF
    exit 0
}

while [ $# -gt 0 ]; do
    case "$1" in
        --version)   VERSION="$2"; shift 2 ;;
        --dir)       INSTALL_DIR="$2"; shift 2 ;;
        --no-verify) NO_VERIFY=true; shift ;;
        -h|--help)   usage ;;
        *)           fail "unknown option: $1 (use --help for usage)" ;;
    esac
done

# --- Cleanup on failure -------------------------------------------------------

WORK_DIR=""
cleanup() {
    if [ -n "${WORK_DIR:-}" ] && [ -d "${WORK_DIR:-}" ]; then
        rm -rf "$WORK_DIR"
    fi
}
trap cleanup EXIT

# --- Platform detection -------------------------------------------------------

detect_platform() {
    local os arch

    os="$(uname -s)"
    case "$os" in
        Linux*)  os="linux" ;;
        Darwin*) os="darwin" ;;
        MINGW*|MSYS*|CYGWIN*) fail "Windows is not directly supported. Use WSL2 instead." ;;
        *)       fail "unsupported operating system: $os" ;;
    esac

    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)             fail "unsupported architecture: $arch" ;;
    esac

    echo "${os}_${arch}"
}

# --- Download helper ----------------------------------------------------------

download() {
    local url="$1" dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL -o "$dest" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        fail "curl or wget is required to download files"
    fi
}

# --- Version resolution -------------------------------------------------------

resolve_version() {
    if [ -n "$VERSION" ]; then
        echo "$VERSION"
        return
    fi

    local api_url="https://api.github.com/repos/${REPO}/releases/latest"
    local response

    if command -v curl >/dev/null 2>&1; then
        response="$(curl -fsSL "$api_url" 2>/dev/null)" || \
            fail "could not fetch latest release from GitHub API. Check your network or use --version."
    elif command -v wget >/dev/null 2>&1; then
        response="$(wget -qO- "$api_url" 2>/dev/null)" || \
            fail "could not fetch latest release from GitHub API. Check your network or use --version."
    else
        fail "curl or wget is required"
    fi

    # Extract tag_name without requiring jq.
    local tag
    tag="$(echo "$response" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name"[[:space:]]*:[[:space:]]*"//;s/".*//')"
    if [ -z "$tag" ]; then
        fail "could not parse latest version from GitHub API response"
    fi

    echo "$tag"
}

# --- Install directory selection ----------------------------------------------

select_install_dir() {
    # Priority 1: explicit --dir flag.
    if [ -n "$INSTALL_DIR" ]; then
        echo "$INSTALL_DIR"
        return
    fi

    # Priority 2: /usr/local/bin if writable or sudo is available.
    if [ -w "/usr/local/bin" ]; then
        echo "/usr/local/bin"
        return
    fi
    if command -v sudo >/dev/null 2>&1; then
        echo "/usr/local/bin"
        return
    fi

    # Priority 3: ~/.local/bin as fallback.
    echo "${HOME}/.local/bin"
}

# Returns 0 (true) if we need sudo to write to the directory.
needs_sudo() {
    local dir="$1"
    [ -d "$dir" ] && [ -w "$dir" ] && return 1
    # Directory doesn't exist — check the parent.
    local parent
    parent="$(dirname "$dir")"
    [ -d "$parent" ] && [ -w "$parent" ] && return 1
    return 0
}

# --- Checksum verification ----------------------------------------------------

verify_checksum() {
    local archive="$1" checksums_file="$2"
    local filename
    filename="$(basename "$archive")"

    # Extract expected checksum for our file.
    local expected
    expected="$(grep "  ${filename}\$" "$checksums_file" | awk '{print $1}')"
    # Try alternative format (no double-space prefix).
    if [ -z "$expected" ]; then
        expected="$(grep "${filename}" "$checksums_file" | awk '{print $1}' | head -1)"
    fi
    if [ -z "$expected" ]; then
        fail "checksum for ${filename} not found in checksums.txt"
    fi

    local actual
    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "$archive" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "$archive" | awk '{print $1}')"
    else
        warn "neither sha256sum nor shasum found, skipping checksum verification"
        return
    fi

    if [ "$expected" != "$actual" ]; then
        fail "SHA256 checksum mismatch! expected=${expected} actual=${actual}"
    fi

    ok "SHA256 checksum verified"
}

# --- PATH management ----------------------------------------------------------

ensure_in_path() {
    local dir="$1"

    # Already in PATH? Nothing to do.
    case ":${PATH}:" in
        *":${dir}:"*) return ;;
    esac

    # Determine which shell rc file to modify.
    local shell_rc=""
    case "$(basename "${SHELL:-bash}")" in
        zsh)  shell_rc="${HOME}/.zshrc" ;;
        bash) shell_rc="${HOME}/.bashrc" ;;
        *)    shell_rc="${HOME}/.profile" ;;
    esac

    local marker="# Added by AI-Box installer"
    local line="export PATH=\"${dir}:\$PATH\"  ${marker}"

    # Skip if we already added the line in a previous run.
    if [ -f "$shell_rc" ] && grep -qF "$marker" "$shell_rc" 2>/dev/null; then
        return
    fi

    printf "\n%s\n" "$line" >> "$shell_rc"
    ok "added ${dir} to PATH in ${shell_rc}"
}

# --- Main install flow --------------------------------------------------------

main() {
    printf "\n${BOLD}AI-Box Installer${NC}\n\n"

    # Step 1: Detect platform.
    info "[1/6] Detecting platform..."
    local platform
    platform="$(detect_platform)"
    ok "$platform"

    # Step 2: Resolve version.
    info "[2/6] Resolving version..."
    local version
    version="$(resolve_version)"
    ok "$version"

    # Step 3: Select install directory.
    info "[3/6] Selecting install directory..."
    local install_dir
    install_dir="$(select_install_dir)"
    ok "$install_dir"

    # Create temporary working directory.
    WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aibox-install.XXXXXX")"

    # Step 4: Download archive (and checksums if verifying).
    info "[4/6] Downloading aibox ${version} for ${platform}..."
    local base_url="https://github.com/${REPO}/releases/download/${version}"
    # GoReleaser uses version without leading 'v' in the archive name.
    local bare_version="${version#v}"
    local archive_name="aibox_${bare_version}_${platform}.tar.gz"
    local archive_path="${WORK_DIR}/${archive_name}"

    download "${base_url}/${archive_name}" "$archive_path" || \
        fail "download failed. Check that version ${version} exists at https://github.com/${REPO}/releases"

    if [ "$NO_VERIFY" != true ]; then
        download "${base_url}/checksums.txt" "${WORK_DIR}/checksums.txt" || \
            fail "could not download checksums.txt for verification"
    fi
    ok "downloaded ${archive_name}"

    # Step 5: Verify checksum.
    info "[5/6] Verifying integrity..."
    if [ "$NO_VERIFY" = true ]; then
        warn "checksum verification skipped (--no-verify)"
    else
        verify_checksum "$archive_path" "${WORK_DIR}/checksums.txt"
    fi

    # Step 6: Extract and install binaries.
    info "[6/6] Installing binaries..."
    local extract_dir="${WORK_DIR}/extracted"
    mkdir -p "$extract_dir"
    tar -xzf "$archive_path" -C "$extract_dir"

    # Create install directory if it doesn't exist.
    local use_sudo=false
    if ! [ -d "$install_dir" ]; then
        if needs_sudo "$install_dir"; then
            use_sudo=true
            sudo mkdir -p "$install_dir"
        else
            mkdir -p "$install_dir"
        fi
    elif needs_sudo "$install_dir"; then
        use_sudo=true
    fi

    local installed=0
    for bin in $BINARIES; do
        local src
        # GoReleaser may place binaries at the top level or in subdirectories.
        src="$(find "$extract_dir" -name "$bin" -type f 2>/dev/null | head -1)"
        if [ -z "$src" ]; then
            warn "binary '${bin}' not found in archive, skipping"
            continue
        fi

        chmod +x "$src"
        if [ "$use_sudo" = true ]; then
            sudo install -m 0755 "$src" "${install_dir}/${bin}"
        else
            install -m 0755 "$src" "${install_dir}/${bin}"
        fi
        installed=$((installed + 1))
    done

    if [ "$installed" -eq 0 ]; then
        fail "no binaries found in the archive. The release may have an unexpected structure."
    fi
    ok "installed ${installed} binaries to ${install_dir}"

    # Add to PATH if not already present.
    ensure_in_path "$install_dir"

    # --- Success summary --------------------------------------------------------

    printf "\n${GREEN}${BOLD}AI-Box ${version} installed successfully!${NC}\n\n"
    printf "  Binaries:  ${install_dir}\n"
    printf "  Version:   ${version}\n\n"
    printf "  ${BOLD}Next steps:${NC}\n"
    printf "    1. aibox setup           # initialize your environment\n"
    printf "    2. aibox doctor           # verify everything works\n"
    printf "    3. aibox start            # launch a sandbox\n\n"

    # Warn if the install dir is not yet in the current session's PATH.
    if ! command -v aibox >/dev/null 2>&1; then
        printf "  ${YELLOW}Note:${NC} Restart your shell or run:\n"
        printf "    export PATH=\"${install_dir}:\$PATH\"\n\n"
    fi
}

main "$@"
