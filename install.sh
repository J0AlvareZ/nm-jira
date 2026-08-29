#!/usr/bin/env bash
set -euo pipefail

REPOSITORY="J0AlvareZ/nm-jira"
GITHUB_API_URL="https://api.github.com/repos/${REPOSITORY}/releases/latest"
GITHUB_RELEASE_URL="https://github.com/${REPOSITORY}/releases/download"

temp_dir=""
version=""
install_dir=""

error() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '%s\n' "$*"
}

usage() {
  cat <<'EOF'
Usage: install.sh [--version <tag>] [--install-dir <dir>]

Download and install nm-jira for Linux or macOS on amd64 or arm64.

Options:
  --version <tag>      Install a specific release tag, including a prerelease.
  --install-dir <dir>  Install into this directory.
  -h, --help           Show this help message.

Without --install-dir, the installer uses /usr/local/bin when it is writable,
otherwise it uses $HOME/.local/bin. It never invokes sudo automatically.

Uninstall:
  rm -f <install-dir>/nm-jira

To roll back, run this installer again with --version <previous-tag>.
EOF
}

cleanup() {
  if [[ -n "$temp_dir" && -d "$temp_dir" ]]; then
    rm -rf -- "$temp_dir"
  fi
}

on_interrupt() {
  cleanup
  exit 1
}

trap cleanup EXIT
trap on_interrupt HUP INT TERM

argument_error() {
  printf 'Error: %s\n\n' "$*" >&2
  usage >&2
  exit 2
}

download_to_file() {
  local url="$1"
  local output="$2"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
  else
    wget -qO "$output" "$url"
  fi
}

download_to_stdout() {
  local url="$1"

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url"
  else
    wget -qO- "$url"
  fi
}

require_dependencies() {
  if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    error "curl or wget is required; install one and run this script again."
  fi
  if ! command -v tar >/dev/null 2>&1; then
    error "tar is required; install it and run this script again."
  fi
  if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
    error "sha256sum or shasum is required; install one and run this script again."
  fi
  if ! command -v mktemp >/dev/null 2>&1; then
    error "mktemp is required; install it and run this script again."
  fi
}

resolve_platform() {
  local kernel
  local machine

  kernel="$(uname -s)"
  machine="$(uname -m)"

  case "$kernel" in
    Linux) goos="linux" ;;
    Darwin) goos="darwin" ;;
    MINGW*|MSYS*|CYGWIN*) error "Windows releases use ZIP archives and are not supported by this Bash installer." ;;
    *) error "unsupported operating system: $kernel" ;;
  esac

  case "$machine" in
    x86_64|amd64) goarch="amd64" ;;
    aarch64|arm64) goarch="arm64" ;;
    *) error "unsupported architecture: $machine" ;;
  esac
}

resolve_version() {
  local release_json

  if [[ -n "$version" ]]; then
    return
  fi

  release_json="$(download_to_stdout "$GITHUB_API_URL")" || error "failed to query the latest nm-jira release."
  version="${release_json#*\"tag_name\": }"
  version="${version#\"}"
  version="${version%%\"*}"
  [[ -n "$version" && "$version" != "$release_json" ]] || error "could not determine tag_name from the latest release response."
}

resolve_install_dir() {
  local home_dir

  if [[ -z "$install_dir" ]]; then
    if [[ -d /usr/local/bin && -w /usr/local/bin ]]; then
      install_dir="/usr/local/bin"
    else
      home_dir="${HOME:-}"
      [[ -n "$home_dir" ]] || error "HOME is not set; pass --install-dir <dir>."
      install_dir="${home_dir}/.local/bin"
    fi
  fi

  mkdir -p "$install_dir" || error "could not create install directory: $install_dir"
  [[ -d "$install_dir" && -w "$install_dir" ]] || error "install directory is not writable: $install_dir"
}

select_checksum() {
  local sums_file="$1"
  local asset_name="$2"
  local checksum
  local filename
  local selected=""

  while read -r checksum filename; do
    filename="${filename#\*}"
    if [[ "$filename" == "$asset_name" ]]; then
      selected="$checksum"
      break
    fi
  done < "$sums_file"

  [[ -n "$selected" ]] || error "SHA256SUMS does not contain an entry for $asset_name"
  printf '%s  %s\n' "$selected" "$asset_name" > "${sums_file}.selected"
  printf '%s\n' "$selected"
}

verify_checksum() {
  local checksum_file="$1"

  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$temp_dir" && sha256sum -c "$checksum_file")
  else
    (cd "$temp_dir" && shasum -a 256 -c "$checksum_file")
  fi
}

install_binary() {
  local source="$1"
  local staged_binary

  staged_binary="$(mktemp "${install_dir}/.nm-jira.XXXXXX")" || error "could not create a temporary file in $install_dir"
  cp "$source" "$staged_binary" || error "could not stage nm-jira in $install_dir"
  chmod 755 "$staged_binary" || error "could not set executable permissions in $install_dir"
  mv -f "$staged_binary" "${install_dir}/nm-jira" || error "could not install nm-jira to $install_dir"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    --version)
      [[ $# -ge 2 && -n "$2" ]] || argument_error "--version requires a tag."
      version="$2"
      shift 2
      ;;
    --install-dir)
      [[ $# -ge 2 && -n "$2" ]] || argument_error "--install-dir requires a directory."
      install_dir="$2"
      shift 2
      ;;
    *)
      argument_error "unknown argument: $1"
      ;;
  esac
done

require_dependencies
resolve_platform
resolve_version
resolve_install_dir

asset_name="nm-jira_${goos}_${goarch}.tar.gz"
asset_url="${GITHUB_RELEASE_URL}/${version}/${asset_name}"
sums_url="${GITHUB_RELEASE_URL}/${version}/SHA256SUMS"
temp_dir="$(mktemp -d)" || error "could not create a temporary directory"

info "Downloading nm-jira ${version} for ${goos}/${goarch}..."
download_to_file "$asset_url" "${temp_dir}/${asset_name}" || error "failed to download $asset_url"
download_to_file "$sums_url" "${temp_dir}/SHA256SUMS" || error "failed to download $sums_url"

checksum="$(select_checksum "${temp_dir}/SHA256SUMS" "$asset_name")"
verify_checksum "SHA256SUMS.selected" || error "checksum validation failed for $asset_name"

tar -xzf "${temp_dir}/${asset_name}" -C "$temp_dir" || error "failed to extract $asset_name"
[[ -f "${temp_dir}/nm-jira" ]] || error "archive does not contain a regular nm-jira file"

install_binary "${temp_dir}/nm-jira"

info "Installed nm-jira ${version} for ${goos}/${goarch}."
info "SHA-256 validated: ${checksum}"
info "Installed to: ${install_dir}/nm-jira"

case ":${PATH:-}:" in
  *":${install_dir}:"*) ;;
  *) info "Add it to PATH with: export PATH=\"${install_dir}:\$PATH\"" ;;
esac
