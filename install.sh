#!/usr/bin/env bash
set -euo pipefail
APP=apcode
REPO="apcode/apcode"
INSTALL_DIR="${APCODE_INSTALL_DIR:-$HOME/.apcode/bin}"

MUTED='\033[0;2m'
RED='\033[0;31m'
GREEN='\033[0;32m'
ORANGE='\033[38;5;214m'
CYAN='\033[0;36m'
NC='\033[0m'

usage() {
    cat <<EOF
APCode Installer

Usage: install.sh [options]

Options:
    -h, --help              Display this help message
    -v, --version <version> Install a specific version (e.g., 0.1.0)
    -b, --binary <path>     Install from a local binary instead of downloading
        --no-modify-path    Don't modify shell config files (.zshrc, .bashrc, etc.)
        --dir <path>        Install directory (default: \$HOME/.apcode/bin)

Examples:
    curl -fsSL https://raw.githubusercontent.com/apcode/apcode/main/install.sh | bash
    curl -fsSL https://raw.githubusercontent.com/apcode/apcode/main/install.sh | bash -s -- --version 0.1.0
    ./install.sh --binary ./apcode
    ./install.sh --dir /usr/local/bin
EOF
}

requested_version=${VERSION:-}
no_modify_path=false
binary_path=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help)
            usage
            exit 0
            ;;
        -v|--version)
            if [[ -n "${2:-}" ]]; then
                requested_version="$2"
                shift 2
            else
                echo -e "${RED}Error: --version requires a version argument${NC}"
                exit 1
            fi
            ;;
        -b|--binary)
            if [[ -n "${2:-}" ]]; then
                binary_path="$2"
                shift 2
            else
                echo -e "${RED}Error: --binary requires a path argument${NC}"
                exit 1
            fi
            ;;
        --no-modify-path)
            no_modify_path=true
            shift
            ;;
        --dir)
            if [[ -n "${2:-}" ]]; then
                INSTALL_DIR="$2"
                shift 2
            else
                echo -e "${RED}Error: --dir requires a path argument${NC}"
                exit 1
            fi
            ;;
        *)
            echo -e "${ORANGE}Warning: Unknown option '$1'${NC}" >&2
            shift
            ;;
    esac
done

mkdir -p "$INSTALL_DIR"

if [ -n "$binary_path" ]; then
    if [ ! -f "$binary_path" ]; then
        echo -e "${RED}Error: Binary not found at ${binary_path}${NC}"
        exit 1
    fi
    specific_version="local"
else
    raw_os=$(uname -s)
    os=$(echo "$raw_os" | tr '[:upper:]' '[:lower:]')
    case "$raw_os" in
      Darwin*) os="darwin" ;;
      Linux*) os="linux" ;;
      MINGW*|MSYS*|CYGWIN*|Windows_NT*) os="windows" ;;
    esac

    arch=$(uname -m)
    if [[ "$arch" == "aarch64" ]]; then
      arch="arm64"
    elif [[ "$arch" == "armv7l" ]]; then
      arch="arm"
    fi
    if [[ "$arch" == "x86_64" ]]; then
      arch="amd64"
    fi
    if [[ "$arch" == "amd64" && "$os" == "darwin" ]]; then
      rosetta_flag=$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)
      if [ "$rosetta_flag" = "1" ]; then
        arch="arm64"
      fi
    fi

    combo="$os-$arch"
    case "$combo" in
      linux-amd64|linux-arm64|darwin-amd64|darwin-arm64|windows-amd64|windows-arm64)
        ;;
      linux-arm)
        echo -e "${ORANGE}Warning: arm 32-bit is experimental, using arm64 fallback check${NC}"
        ;;
      *)
        echo -e "${RED}Unsupported OS/Arch: $os/$arch${NC}"
        echo -e "${MUTED}Supported: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64, windows-amd64, windows-arm64${NC}"
        exit 1
        ;;
    esac

    ext=""
    if [ "$os" = "windows" ]; then
      ext=".exe"
    fi

    archive_ext=".tar.gz"
    if [ "$os" = "windows" ]; then
      archive_ext=".zip"
    fi

    # Determine filename pattern produced by GoReleaser
    # apcode_<version>_<os>_<arch>.tar.gz  or apcode_<os>_<arch>.zip
    if [ -z "$requested_version" ]; then
        if command -v curl >/dev/null 2>&1; then
            specific_version=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null | sed -n 's/.*"tag_name": *"v\([^"]*\)".*/\1/p' | head -n1)
        fi
        if [[ -z "${specific_version:-}" ]]; then
            echo -e "${RED}Failed to fetch latest version${NC}"
            echo -e "${MUTED}Try: curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash -s -- --version 0.1.0${NC}"
            echo -e "${MUTED}Releases: https://github.com/${REPO}/releases${NC}"
            exit 1
        fi
        url="https://github.com/${REPO}/releases/download/v${specific_version}/apcode_${specific_version}_${os}_${arch}${archive_ext}"
        # Fallback filename without version prefix for older releases
        # apcode-linux-amd64.tar.gz style
        fallback_url="https://github.com/${REPO}/releases/download/v${specific_version}/apcode-${os}-${arch}${archive_ext}"
        if [ "$os" = "windows" ]; then
            fallback_url="https://github.com/${REPO}/releases/download/v${specific_version}/apcode-${os}-${arch}.zip"
        fi
    else
        requested_version="${requested_version#v}"
        specific_version=$requested_version
        url="https://github.com/${REPO}/releases/download/v${specific_version}/apcode_${specific_version}_${os}_${arch}${archive_ext}"
        fallback_url="https://github.com/${REPO}/releases/download/v${specific_version}/apcode-${os}-${arch}${archive_ext}"
        if [ "$os" = "windows" ]; then
            fallback_url="https://github.com/${REPO}/releases/download/v${specific_version}/apcode-${os}-${arch}.zip"
        fi

        if command -v curl >/dev/null 2>&1; then
            http_status=$(curl -sI -o /dev/null -w "%{http_code}" "https://github.com/${REPO}/releases/tag/v${requested_version}" 2>/dev/null || echo "000")
            if [ "$http_status" = "404" ]; then
                echo -e "${RED}Error: Release v${requested_version} not found${NC}"
                echo -e "${MUTED}Available releases: https://github.com/${REPO}/releases${NC}"
                exit 1
            fi
        fi
    fi
fi

print_message() {
    local level=$1
    local message=$2
    local color=""
    case $level in
        info) color="${NC}" ;;
        warning) color="${ORANGE}" ;;
        error) color="${RED}" ;;
        success) color="${GREEN}" ;;
    esac
    echo -e "${color}${message}${NC}"
}

check_existing() {
    if command -v apcode >/dev/null 2>&1; then
        apcode_path=$(which apcode 2>/dev/null || command -v apcode)
        installed_version=$(apcode --version 2>/dev/null | awk '{print $2}' || echo "")
        if [[ -n "$installed_version" && "$installed_version" == "$specific_version" ]]; then
            print_message info "${MUTED}Version ${NC}$specific_version${MUTED} already installed at ${NC}$apcode_path"
            # Ensure it's in our install dir
            if [[ "$apcode_path" != "$INSTALL_DIR/apcode"* ]]; then
                print_message info "${MUTED}Installed at different location, continuing to install to ${NC}$INSTALL_DIR"
            else
                exit 0
            fi
        elif [[ -n "$installed_version" ]]; then
            print_message info "${MUTED}Installed version: ${NC}$installed_version ${MUTED}-> upgrading to ${NC}$specific_version"
        fi
    fi
}

download_and_install() {
    print_message info "\n${MUTED}Installing ${NC}apcode ${MUTED}version: ${NC}$specific_version ${MUTED}for ${NC}$os/$arch"
    # Use mktemp for safer temporary directory (avoid predictable PID-based path)
    if command -v mktemp >/dev/null 2>&1; then
        tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/apcode_install_XXXXXX" 2>/dev/null || mktemp -d 2>/dev/null)
        if [ -z "$tmp_dir" ] || [ ! -d "$tmp_dir" ]; then
            tmp_dir="${TMPDIR:-/tmp}/apcode_install_$$"
            mkdir -p "$tmp_dir"
        fi
    else
        tmp_dir="${TMPDIR:-/tmp}/apcode_install_$$"
        mkdir -p "$tmp_dir"
    fi
    tmp_file="$tmp_dir/apcode${archive_ext}"

    # Check deps
    if [ "$os" = "linux" ]; then
        if ! command -v tar >/dev/null 2>&1; then
             echo -e "${RED}Error: 'tar' is required but not installed.${NC}"
             exit 1
        fi
    else
        if ! command -v unzip >/dev/null 2>&1; then
            echo -e "${RED}Error: 'unzip' is required but not installed.${NC}"
            exit 1
        fi
    fi
    if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
        echo -e "${RED}Error: 'curl' or 'wget' is required${NC}"
        exit 1
    fi

    # Download with curl, fallback wget
    download_success=false
    successful_url=""
    for dl_url in "$url" "$fallback_url"; do
        echo -e "${MUTED}Downloading ${NC}$dl_url"
        if command -v curl >/dev/null 2>&1; then
            if curl -fsSL -o "$tmp_file" "$dl_url" 2>/dev/null; then
                download_success=true
                successful_url="$dl_url"
                break
            fi
            # try with progress
            if curl -# -L -o "$tmp_file" "$dl_url" 2>&1; then
                download_success=true
                successful_url="$dl_url"
                break
            fi
        elif command -v wget >/dev/null 2>&1; then
            if wget -qO "$tmp_file" "$dl_url" 2>/dev/null; then
                download_success=true
                successful_url="$dl_url"
                break
            fi
        fi
    done

    if [ "$download_success" != "true" ]; then
        echo -e "${RED}Failed to download apcode from GitHub releases${NC}"
        echo -e "${MUTED}Tried: ${NC}$url"
        echo -e "${MUTED}  and: ${NC}$fallback_url"
        echo -e "${MUTED}Check: https://github.com/${REPO}/releases${NC}"
        echo -e "${MUTED}Or install from source: ${NC}go install ./cmd/apcode"
        rm -rf "$tmp_dir"
        exit 1
    fi

    # Optional checksum verification (GoReleaser produces checksums.txt)
    checksum_url="https://github.com/${REPO}/releases/download/v${specific_version}/checksums.txt"
    if command -v curl >/dev/null 2>&1 && [ -n "$successful_url" ]; then
        if curl -fsSL -o "$tmp_dir/checksums.txt" "$checksum_url" 2>/dev/null; then
            archive_name=$(basename "$successful_url")
            # Try sha256sum (Linux) then shasum (macOS)
            if command -v sha256sum >/dev/null 2>&1; then
                if (cd "$tmp_dir" && grep -F "$archive_name" checksums.txt | sha256sum -c - >/dev/null 2>&1); then
                    print_message success "✓ Checksum verified ($archive_name)"
                else
                    print_message warning "Checksum verification failed or entry not found — continuing (HTTPS transport is still secure)"
                fi
            elif command -v shasum >/dev/null 2>&1; then
                if (cd "$tmp_dir" && grep -F "$archive_name" checksums.txt | shasum -a 256 -c - >/dev/null 2>&1); then
                    print_message success "✓ Checksum verified ($archive_name)"
                else
                    print_message warning "Checksum verification skipped (shasum failed)"
                fi
            else
                print_message info "${MUTED}No sha256sum/shasum found, skipping checksum verification (HTTPS provides transport security)${NC}"
            fi
        else
            print_message info "${MUTED}No checksums.txt for v${specific_version}, skipping verification${NC}"
        fi
    fi

    # Extract
    if [ "$os" = "linux" ] || [[ "$archive_ext" == ".tar.gz" ]]; then
        tar -xzf "$tmp_file" -C "$tmp_dir"
    else
        unzip -q "$tmp_file" -d "$tmp_dir"
    fi

    # Find binary
    bin_src=""
    if [ -f "$tmp_dir/apcode${ext}" ]; then
        bin_src="$tmp_dir/apcode${ext}"
    elif [ -f "$tmp_dir/apcode" ]; then
        bin_src="$tmp_dir/apcode"
    else
        # Search recursive (goreleaser puts binary at root, but be safe)
        bin_src=$(find "$tmp_dir" -type f -name "apcode*" -perm -111 2>/dev/null | head -n1 || find "$tmp_dir" -type f -name "apcode*" 2>/dev/null | head -n1)
    fi

    if [ -z "$bin_src" ] || [ ! -f "$bin_src" ]; then
        echo -e "${RED}Extracted archive does not contain apcode binary${NC}"
        echo -e "${MUTED}Contents of $tmp_dir:${NC}"
        ls -R "$tmp_dir" 2>/dev/null || ls -la "$tmp_dir"
        rm -rf "$tmp_dir"
        exit 1
    fi

    mv "$bin_src" "$INSTALL_DIR/apcode${ext}"
    chmod 755 "$INSTALL_DIR/apcode${ext}"
    # On windows, also provide without .exe alias if needed? No.
    rm -rf "$tmp_dir"
    print_message success "✓ Installed apcode to $INSTALL_DIR/apcode${ext}"
}

install_from_binary() {
    print_message info "\n${MUTED}Installing ${NC}apcode ${MUTED}from: ${NC}$binary_path"
    ext=""
    if [[ "$binary_path" == *.exe ]]; then
        ext=".exe"
    fi
    # Detect if source is windows binary vs unix
    cp "$binary_path" "$INSTALL_DIR/apcode${ext}"
    # Also copy without ext for unix convenience if exe supplied?
    if [[ "$ext" == ".exe" && "$(uname -s)" != *MINGW* && "$(uname -s)" != *MSYS* ]]; then
        cp "$binary_path" "$INSTALL_DIR/apcode" 2>/dev/null || true
        chmod 755 "$INSTALL_DIR/apcode" 2>/dev/null || true
    fi
    chmod 755 "$INSTALL_DIR/apcode${ext}"
    print_message success "✓ Installed apcode to $INSTALL_DIR/apcode${ext}"
}

if [ -n "$binary_path" ]; then
    install_from_binary
else
    check_existing
    download_and_install
fi

# Verify
if [ -x "$INSTALL_DIR/apcode" ] || [ -x "$INSTALL_DIR/apcode.exe" ]; then
    bin_to_check="$INSTALL_DIR/apcode"
    if [ ! -x "$bin_to_check" ] && [ -x "$INSTALL_DIR/apcode.exe" ]; then
        bin_to_check="$INSTALL_DIR/apcode.exe"
    fi
    ver=$("$bin_to_check" --version 2>/dev/null || echo "unknown")
    print_message success "✓ Verified: $ver"
fi

add_to_path() {
    local config_file=$1
    local command=$2
    if grep -Fxq "$command" "$config_file" 2>/dev/null; then
        print_message info "Command already exists in $config_file, skipping write."
    elif [[ -w $config_file ]]; then
        echo -e "\n# apcode" >> "$config_file"
        echo "$command" >> "$config_file"
        print_message info "${MUTED}Successfully added ${NC}apcode ${MUTED}to \$PATH in ${NC}$config_file"
    else
        print_message warning "Manually add the directory to $config_file (or similar):"
        print_message info "  $command"
    fi
}

XDG_CONFIG_HOME=${XDG_CONFIG_HOME:-$HOME/.config}
current_shell=$(basename "${SHELL:-bash}")
case $current_shell in
    fish)
        config_files="$HOME/.config/fish/config.fish"
    ;;
    zsh)
        config_files="${ZDOTDIR:-$HOME}/.zshrc ${ZDOTDIR:-$HOME}/.zshenv $XDG_CONFIG_HOME/zsh/.zshrc $XDG_CONFIG_HOME/zsh/.zshenv"
    ;;
    bash)
        config_files="$HOME/.bashrc $HOME/.bash_profile $HOME/.profile $XDG_CONFIG_HOME/bash/.bashrc $XDG_CONFIG_HOME/bash/.bash_profile"
    ;;
    ash|sh)
        config_files="$HOME/.ashrc $HOME/.profile /etc/profile"
    ;;
    *)
        config_files="$HOME/.bashrc $HOME/.bash_profile $XDG_CONFIG_HOME/bash/.bashrc $XDG_CONFIG_HOME/bash/.bash_profile"
    ;;
esac

if [[ "$no_modify_path" != "true" ]]; then
    config_file=""
    for file in $config_files; do
        if [[ -f $file ]]; then
            config_file=$file
            break
        fi
    done
    if [[ -z $config_file ]]; then
        print_message warning "No config file found for $current_shell. You may need to manually add to PATH:"
        print_message info "  export PATH=\"$INSTALL_DIR:\$PATH\""
    elif [[ ":$PATH:" != *":$INSTALL_DIR:"* ]]; then
        case $current_shell in
            fish)
                add_to_path "$config_file" "fish_add_path $INSTALL_DIR"
            ;;
            zsh|bash|ash|sh|*)
                add_to_path "$config_file" "export PATH=\"$INSTALL_DIR:\$PATH\""
            ;;
        esac
        print_message info "${MUTED}Restart your shell or run: ${NC}export PATH=\"$INSTALL_DIR:\$PATH\""
    else
        print_message info "${MUTED}Already in PATH: ${NC}$INSTALL_DIR"
    fi
fi

# GitHub Actions
if [ -n "${GITHUB_ACTIONS-}" ] && [ "${GITHUB_ACTIONS}" == "true" ] && [ -n "${GITHUB_PATH-}" ]; then
    echo "$INSTALL_DIR" >> "$GITHUB_PATH"
    print_message info "Added $INSTALL_DIR to \$GITHUB_PATH"
fi

echo -e ""
echo -e "${CYAN} █████╗ ██████╗  ██████╗ ██████╗ ██████╗ ███████╗${NC}"
echo -e "${CYAN}██╔══██╗██╔══██╗██╔════╝██╔═══██╗██╔══██╗██╔════╝${NC}"
echo -e "${CYAN}███████║██████╔╝██║     ██║   ██║██║  ██║█████╗  ${NC}"
echo -e "${CYAN}██╔══██║██╔═══╝ ██║     ██║   ██║██║  ██║██╔══╝  ${NC}"
echo -e "${CYAN}██║  ██║██║     ╚██████╗╚██████╔╝██████╔╝███████╗${NC}"
echo -e "${CYAN}╚═╝  ╚═╝╚═╝      ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝${NC}"
echo -e ""
echo -e "${MUTED}APCode is offline-first. Everything runs locally.${NC}"
echo -e ""
echo -e "  ${GREEN}apcode${NC}                 ${MUTED}# welcome + system info${NC}"
echo -e "  ${GREEN}apcode benchmark${NC}       ${MUTED}# run hardware benchmarks${NC}"
echo -e "  ${GREEN}apcode models${NC}          ${MUTED}# list models${NC}"
echo -e "  ${GREEN}apcode recommend${NC}       ${MUTED}# recommend model for this hardware${NC}"
echo -e "  ${GREEN}apcode context${NC}         ${MUTED}# show project context${NC}"
echo -e "  ${GREEN}apcode search <query>${NC}  ${MUTED}# search code locally${NC}"
echo -e "  ${GREEN}apcode runtime${NC}         ${MUTED}# check runtime${NC}"
echo -e "  ${GREEN}apcode infer \"prompt\"${NC} ${MUTED}# run inference (if runtime+model)${NC}"
echo -e ""
echo -e "${MUTED}Docs: https://github.com/${REPO}#readme${NC}"
echo -e "${MUTED}If 'apcode' not found, restart shell or: export PATH=\"\$HOME/.apcode/bin:\$PATH\"${NC}"
echo -e ""
