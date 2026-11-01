#!/bin/sh
# tuipe installer for macOS and Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/jiripisa/tuipe/main/install.sh | sh
#
# Environment:
#   TUIPE_INSTALL_DIR  target folder (default: $HOME/.local/bin)
#   TUIPE_API_BASE     GitHub API base (default: https://api.github.com)
#
# Everything lives in main(), called on the last line, so a truncated
# download cannot run half a script.
set -eu

REPO="jiripisa/tuipe"
TMP=""
STAGED=""
OS=""
ARCH=""

die() {
  printf 'Chyba: %s\n' "$*" >&2
  exit 1
}

# fetch is curl that speaks HTTPS only. TUIPE_API_BASE names a test server, which
# is plain HTTP.
fetch() {
  if [ -n "${TUIPE_API_BASE:-}" ]; then
    curl "$@"
  else
    curl --proto '=https' "$@"
  fi
}

cleanup() {
  if [ -n "$STAGED" ]; then
    rm -f "$STAGED"
  fi
  if [ -n "$TMP" ] && [ -d "$TMP" ]; then
    rm -rf "$TMP"
  fi
}

# Sets OS and ARCH in GoReleaser naming. Not run in a subshell, so die works.
detect_platform() {
  case "$(uname -s)" in
    Darwin) OS=darwin ;;
    Linux) OS=linux ;;
    *) die "Nepodporovaný operační systém: $(uname -s). Podporujeme macOS a Linux (Windows: install.ps1)." ;;
  esac
  case "$(uname -m)" in
    x86_64 | amd64) ARCH=amd64 ;;
    aarch64 | arm64) ARCH=arm64 ;;
    *) die "Nepodporovaná architektura: $(uname -m). Podporujeme arm64 a amd64." ;;
  esac
  # An Intel shell under Rosetta on an Apple silicon Mac: install the native build.
  if [ "$OS" = darwin ] && [ "$ARCH" = amd64 ]; then
    if [ "$(sysctl -n hw.optional.arm64 2>/dev/null || true)" = 1 ]; then
      ARCH=arm64
    fi
  fi
}

# SHA-256 of a file, hex only.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    die "Chybí sha256sum i shasum, nelze ověřit kontrolní součet."
  fi
}

# Prints args as one shell word, quoted only when needed (for pasting).
shell_quote() {
  case "$1" in
    *[!A-Za-z0-9_./:@%+=-]*)
      printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
      ;;
    *) printf '%s' "$1" ;;
  esac
}

# True if $1 is a PATH entry, with or without trailing slashes.
on_path() {
  want="${1%/}"
  old_ifs="$IFS"
  IFS=:
  for entry in $PATH; do
    while [ "${entry%/}" != "$entry" ] && [ -n "${entry%/}" ]; do
      entry="${entry%/}"
    done
    if [ "$entry" = "$want" ]; then
      IFS="$old_ifs"
      return 0
    fi
  done
  IFS="$old_ifs"
  return 1
}

# Prints the line to add to the user's shell startup file.
path_help() {
  dir="$1"
  line="export PATH=\"$dir:\$PATH\""
  case "$dir" in
    "$HOME"/.local/bin) line="export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
  esac
  # shellcheck disable=SC2088
  case "${SHELL:-}" in
    */zsh) rc="~/.zshrc" ;;
    */bash)
      if [ "$OS" = darwin ]; then rc="~/.bash_profile"; else rc="~/.bashrc"; fi
      ;;
    */fish)
      rc="~/.config/fish/config.fish"
      line="fish_add_path \"$dir\""
      ;;
    *) rc="~/.profile" ;;
  esac
  printf '\nSložka %s není na PATH.\nPřidejte do %s tento řádek:\n\n  %s\n\nPak otevřete nový terminál.\n\n' "$dir" "$rc" "$line"
}

main() {
  command -v curl >/dev/null 2>&1 || die "Chybí curl."
  api="${TUIPE_API_BASE:-https://api.github.com}"
  dir="${TUIPE_INSTALL_DIR:-$HOME/.local/bin}"

  detect_platform
  os="$OS"
  arch="$ARCH"

  TMP="$(mktemp -d "${TMPDIR:-/tmp}/tuipe-install.XXXXXX")" || die "Nepodařilo se vytvořit dočasnou složku."
  trap cleanup EXIT
  trap 'exit 130' INT
  trap 'exit 143' TERM HUP

  printf 'Zjišťuji nejnovější verzi tuipe...\n'
  status="$(fetch -sSL --connect-timeout 10 --max-time 30 -H 'Accept: application/vnd.github+json' \
    -o "$TMP/release.json" -w '%{http_code}' "$api/repos/$REPO/releases/latest")" ||
    die "Nelze se spojit s GitHubem. Zkontrolujte připojení k internetu a zkuste to znovu."
  case "$status" in
    200) ;;
    404) die "Zatím neexistuje žádné vydání ke stažení." ;;
    403 | 429) die "GitHub dočasně odmítl dotaz (příliš mnoho požadavků). Zkuste to za chvíli." ;;
    *) die "GitHub odpověděl nečekaně (HTTP $status)." ;;
  esac
  # One JSON field per line, then plain text tools; no jq needed.
  # shellcheck disable=SC2020
  tr ',{}[]' '\n\n\n\n\n' <"$TMP/release.json" >"$TMP/release.lines"
  tag="$(sed -n 's/^[[:space:]]*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$TMP/release.lines" | head -n 1)"
  [ -n "$tag" ] || die "V odpovědi chybí číslo verze."
  version="${tag#v}"

  archive="tuipe_${version}_${os}_${arch}.tar.gz"
  urls="$(sed -n 's/^[[:space:]]*"browser_download_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$TMP/release.lines")"
  archive_url="$(printf '%s\n' "$urls" | grep "/$archive\$" | head -n 1 || true)"
  sums_url="$(printf '%s\n' "$urls" | grep '/checksums\.txt$' | head -n 1 || true)"
  [ -n "$archive_url" ] || die "Verze $tag nemá soubor $archive."
  [ -n "$sums_url" ] || die "Verze $tag nemá soubor checksums.txt."

  printf 'Stahuji tuipe %s (%s/%s)...\n' "$tag" "$os" "$arch"
  fetch -fsSL --connect-timeout 10 --max-time 300 "$archive_url" -o "$TMP/$archive" || die "Stažení $archive se nezdařilo."
  fetch -fsSL --connect-timeout 10 --max-time 60 "$sums_url" -o "$TMP/checksums.txt" || die "Stažení checksums.txt se nezdařilo."

  want="$(awk -v n="$archive" '{ f = $2; sub(/^\*/, "", f); if (f == n) { print tolower($1); exit } }' "$TMP/checksums.txt")"
  [ -n "$want" ] || die "V checksums.txt chybí kontrolní součet pro $archive."
  got="$(sha256_of "$TMP/$archive")"
  [ "$got" = "$want" ] || die "Kontrolní součet nesouhlasí, nic jsem neinstaloval."

  mkdir "$TMP/x"
  tar -xzf "$TMP/$archive" -C "$TMP/x" tuipe || die "Archiv $archive je poškozený."
  [ -f "$TMP/x/tuipe" ] || die "Archiv $archive neobsahuje program tuipe."
  chmod 755 "$TMP/x/tuipe"

  if [ -d "$dir/tuipe" ]; then
    die "$dir/tuipe je složka, ne program. Přesuňte ji a spusťte instalaci znovu."
  fi
  mkdir -p "$dir" || die "Nepodařilo se vytvořit složku $dir."
  # Copy next to the target first, then rename: the install is all or nothing.
  STAGED="$dir/.tuipe.new.$$"
  cp "$TMP/x/tuipe" "$STAGED" || die "Nepodařilo se zapsat do $dir."
  mv -f "$STAGED" "$dir/tuipe" || die "Nepodařilo se nainstalovat do $dir."
  STAGED=""

  # Only macOS, and only if the flag is there (curl adds none).
  if [ "$os" = darwin ] && xattr -p com.apple.quarantine "$dir/tuipe" >/dev/null 2>&1; then
    xattr -d com.apple.quarantine "$dir/tuipe" 2>/dev/null || true
  fi

  run="tuipe"
  if ! on_path "$dir"; then
    path_help "$dir"
    run="$(shell_quote "$dir/tuipe")"
  fi
  printf 'tuipe nainstalováno. Spusťte: %s\n' "$run"
}

main "$@"
