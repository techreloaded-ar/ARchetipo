#!/usr/bin/env bash
set -euo pipefail

# ─── AIRchetipo Installer ─────────────────────────────────────────────────────
# Installs AIRchetipo skills + config for Claude Code, Codex, Gemini CLI, OpenCode, GitHub Copilot
# Usage: curl -fsSL https://raw.githubusercontent.com/techreloaded-ar/AIRchetipo/main/install.sh | bash
#        ./install.sh [--local] [--cleanup] [--help]
#   --local    Installs from local ./skills/ folder instead of GitHub
#   --cleanup  Removes installed skills from selected tools
#   --help     Shows this help message
# ──────────────────────────────────────────────────────────────────────────────

REPO_ZIP="https://github.com/techreloaded-ar/AIRchetipo/archive/refs/heads/main.zip"
SKILL_NAMES=("airchetipo-autopilot" "airchetipo-design" "airchetipo-implement" "airchetipo-inception" "airchetipo-plan")

# ─── Help ─────────────────────────────────────────────────────────────────────
show_help() {
  cat <<'HELP'
AIRchetipo Installer

Usage:
  curl -fsSL https://raw.githubusercontent.com/techreloaded-ar/AIRchetipo/main/install.sh | bash
  ./install.sh [--local] [--cleanup] [--help]

Flags:
  --local    Install from local ./skills/ folder instead of downloading from GitHub
  --cleanup  Remove installed skills from selected tools
  --help     Show this help message

Skills installed:
  airchetipo-autopilot
  airchetipo-design
  airchetipo-implement
  airchetipo-inception
  airchetipo-plan

Configuration:
  .airchetipo/config.yaml is created with the selected backend (file or github).

Supported tools:
  Claude Code, Codex, Gemini CLI, OpenCode, GitHub Copilot
HELP
}

# ─── Colors ───────────────────────────────────────────────────────────────────
BOLD='\033[1m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
DIM='\033[2m'
RESET='\033[0m'

# ─── Tool definitions ─────────────────────────────────────────────────────────
TOOL_NAMES=("Claude Code" "Codex" "Gemini CLI" "OpenCode" "GitHub Copilot")
TOOL_PATHS=(".claude/skills" ".agents/skills" ".gemini/skills" ".opencode/skills" ".github/skills")
TOOL_COUNT=${#TOOL_NAMES[@]}

# ─── Cleanup ──────────────────────────────────────────────────────────────────
TMPDIR_INSTALL=""
cleanup() {
  [[ -n "$TMPDIR_INSTALL" && -d "$TMPDIR_INSTALL" ]] && rm -rf "$TMPDIR_INSTALL" || true
}
trap cleanup EXIT

# ─── Install for a specific tool ──────────────────────────────────────────────
install_for_tool() {
  local tool_index="$1"
  local source_dir="$2"
  local tool_name="${TOOL_NAMES[$tool_index]}"
  local tool_path="${TOOL_PATHS[$tool_index]}"

  for skill_name in "${SKILL_NAMES[@]}"; do
    mkdir -p "$tool_path"
    cp -rf "$source_dir/$skill_name" "$tool_path/"
  done

  echo ""
  printf " ${GREEN}✓${RESET} ${BOLD}%s${RESET} ${DIM}→ %s${RESET}\n" "$tool_name" "$tool_path"
  for skill_name in "${SKILL_NAMES[@]}"; do
    printf "   ${DIM}%s/${RESET}\n" "$skill_name"
  done
}

# ─── Cleanup for a specific tool ──────────────────────────────────────────────
cleanup_for_tool() {
  local tool_index="$1"
  local tool_name="${TOOL_NAMES[$tool_index]}"
  local tool_path="${TOOL_PATHS[$tool_index]}"

  local removed=0
  for skill_name in "${SKILL_NAMES[@]}"; do
    local target="$tool_path/$skill_name"
    if [[ -d "$target" ]]; then
      rm -rf "$target"
      removed=1
    fi
  done

  echo ""
  if [[ $removed -eq 1 ]]; then
    printf " ${GREEN}✓${RESET} ${BOLD}%s${RESET} ${DIM}→ rimosso da %s${RESET}\n" "$tool_name" "$tool_path"
  else
    printf " ${YELLOW}–${RESET} ${BOLD}%s${RESET} ${DIM}→ nessuna skill trovata in %s${RESET}\n" "$tool_name" "$tool_path"
  fi
}

# ─── Interactive multi-select menu ────────────────────────────────────────────
interactive_menu() {
  local selected=()
  local cursor=0

  for ((i = 0; i < TOOL_COUNT; i++)); do
    selected+=(0) # all deselected by default
  done

  # Reconnect stdin from tty for pipe mode (curl | bash)
  if [[ ! -t 0 ]]; then
    exec < /dev/tty
  fi

  # Check if we have a real terminal
  if [[ ! -t 0 ]]; then
    fallback_menu
    return
  fi

  # Hide cursor
  printf '\033[?25l'
  # Restore cursor on exit from this function
  trap 'printf "\033[?25h"' RETURN

  local draw_menu
  draw_menu() {
    # Move cursor up to redraw (except first draw)
    if [[ "${1:-}" == "redraw" ]]; then
      printf "\033[%dA" "$TOOL_COUNT"
    fi

    for ((i = 0; i < TOOL_COUNT; i++)); do
      local checkbox
      if [[ ${selected[$i]} -eq 1 ]]; then
        checkbox="${GREEN}[x]${RESET}"
      else
        checkbox="[ ]"
      fi

      local line
      if [[ $i -eq $cursor ]]; then
        line="${CYAN}❯${RESET} ${checkbox} ${BOLD}${TOOL_NAMES[$i]}${RESET} ${DIM}(${TOOL_PATHS[$i]})${RESET}"
      else
        line="  ${checkbox} ${TOOL_NAMES[$i]} ${DIM}(${TOOL_PATHS[$i]})${RESET}"
      fi

      printf "\r\033[K%b\n" "$line"
    done
    printf "\r\033[K${DIM}  ↑↓ navigate  SPACE toggle  ENTER confirm${RESET}"
  }

  draw_menu "first"

  while true; do
    # Read single keypress
    IFS= read -rsn1 key

    case "$key" in
      $'\x1b') # Escape sequence
        read -rsn2 seq
        case "$seq" in
          '[A') # Up arrow
            ((cursor > 0)) && ((cursor--))
            ;;
          '[B') # Down arrow
            ((cursor < TOOL_COUNT - 1)) && ((cursor++))
            ;;
        esac
        ;;
      ' ') # Space — toggle
        if [[ ${selected[$cursor]} -eq 1 ]]; then
          selected[$cursor]=0
        else
          selected[$cursor]=1
        fi
        ;;
      '') # Enter — confirm
        printf "\n\n"
        # Return selected indices
        SELECTED_TOOLS=()
        for ((i = 0; i < TOOL_COUNT; i++)); do
          if [[ ${selected[$i]} -eq 1 ]]; then
            SELECTED_TOOLS+=("$i")
          fi
        done
        return
        ;;
    esac

    draw_menu "redraw"
  done
}

# ─── Fallback numbered menu for non-interactive terminals ─────────────────────
fallback_menu() {
  echo ""
  for ((i = 0; i < TOOL_COUNT; i++)); do
    printf "  %d) %s (%s)\n" "$((i + 1))" "${TOOL_NAMES[$i]}" "${TOOL_PATHS[$i]}"
  done
  echo ""
  printf "Enter tool numbers separated by spaces (e.g. 1 2 3), or 'all': "
  read -r choices

  SELECTED_TOOLS=()
  if [[ "$choices" == "all" ]]; then
    for ((i = 0; i < TOOL_COUNT; i++)); do
      SELECTED_TOOLS+=("$i")
    done
  else
    for choice in $choices; do
      local idx=$((choice - 1))
      if [[ $idx -ge 0 && $idx -lt $TOOL_COUNT ]]; then
        SELECTED_TOOLS+=("$idx")
      fi
    done
  fi
}

# ─── Backend selection (radio-button, single choice) ─────────────────────────
BACKEND_OPTIONS=("file" "github")
BACKEND_DESCRIPTIONS=("backlog e planning come file Markdown locali" "backlog e planning su GitHub Projects v2 — richiede GitHub CLI")

select_backend() {
  local cursor=0

  # Reconnect stdin from tty for pipe mode (curl | bash)
  if [[ ! -t 0 ]]; then
    exec < /dev/tty
  fi

  if [[ ! -t 0 ]]; then
    fallback_backend
    return
  fi

  printf '\033[?25l'
  trap 'printf "\033[?25h"' RETURN

  local draw_backend
  draw_backend() {
    if [[ "${1:-}" == "redraw" ]]; then
      printf "\033[%dA" "${#BACKEND_OPTIONS[@]}"
    fi

    for ((i = 0; i < ${#BACKEND_OPTIONS[@]}; i++)); do
      local radio
      if [[ $i -eq $cursor ]]; then
        radio="${GREEN}(x)${RESET}"
      else
        radio="( )"
      fi

      local line
      if [[ $i -eq $cursor ]]; then
        line="${CYAN}❯${RESET} ${radio} ${BOLD}${BACKEND_OPTIONS[$i]}${RESET}  ${DIM}${BACKEND_DESCRIPTIONS[$i]}${RESET}"
      else
        line="  ${radio} ${BACKEND_OPTIONS[$i]}  ${DIM}${BACKEND_DESCRIPTIONS[$i]}${RESET}"
      fi

      printf "\r\033[K%b\n" "$line"
    done
    printf "\r\033[K${DIM}  ↑↓ navigate  ENTER confirm${RESET}"
  }

  draw_backend "first"

  while true; do
    IFS= read -rsn1 key

    case "$key" in
      $'\x1b')
        read -rsn2 seq
        case "$seq" in
          '[A') ((cursor > 0)) && ((cursor--)) ;;
          '[B') ((cursor < ${#BACKEND_OPTIONS[@]} - 1)) && ((cursor++)) ;;
        esac
        ;;
      '')
        printf "\n\n"
        SELECTED_BACKEND="${BACKEND_OPTIONS[$cursor]}"
        return
        ;;
    esac

    draw_backend "redraw"
  done
}

fallback_backend() {
  echo ""
  for ((i = 0; i < ${#BACKEND_OPTIONS[@]}; i++)); do
    printf "  %d) %s  (%s)\n" "$((i + 1))" "${BACKEND_OPTIONS[$i]}" "${BACKEND_DESCRIPTIONS[$i]}"
  done
  echo ""
  printf "Select backend [1]: "
  read -r choice

  if [[ "$choice" == "2" ]]; then
    SELECTED_BACKEND="github"
  else
    SELECTED_BACKEND="file"
  fi
}

# ─── Install config ──────────────────────────────────────────────────────────
install_config() {
  local source_dir="$1"
  local backend="$2"
  local config_dir=".airchetipo"
  local config_file="$config_dir/config.yaml"

  # Determine source config path
  local source_config=""
  if [[ -f "$source_dir/../config.yaml" ]]; then
    source_config="$source_dir/../config.yaml"
  else
    echo ""
    printf "  ${YELLOW}–${RESET} ${DIM}config.yaml non trovato nella source, skip${RESET}\n"
    return
  fi

  # Check if config already exists
  if [[ -f "$config_file" ]]; then
    printf "\n  ${YELLOW}!${RESET} ${BOLD}.airchetipo/config.yaml${RESET} esiste già. Sovrascrivere? [s/N] "
    read -r answer < /dev/tty
    if [[ "$answer" != "s" && "$answer" != "S" && "$answer" != "y" && "$answer" != "Y" ]]; then
      printf "  ${DIM}Config non modificato${RESET}\n"
      return
    fi
  fi

  mkdir -p "$config_dir"
  cp -f "$source_config" "$config_file"

  # Update backend value
  if command -v sed &>/dev/null; then
    sed -i.bak "s/^backend:.*/backend: $backend/" "$config_file" && rm -f "$config_file.bak"
  fi

  printf "\n  ${GREEN}✓${RESET} ${BOLD}.airchetipo/config.yaml${RESET} ${DIM}(backend: %s)${RESET}\n" "$backend"
}

# ─── Main ─────────────────────────────────────────────────────────────────────
main() {
  # Parse arguments
  local use_local=0
  local do_cleanup=0
  for arg in "$@"; do
    case "$arg" in
      --help|-h) show_help; exit 0 ;;
      --local) use_local=1 ;;
      --cleanup) do_cleanup=1 ;;
    esac
  done

  echo ""
  printf "${BOLD}${CYAN}  AIRchetipo Installer${RESET}\n"

  if [[ $do_cleanup -eq 1 ]]; then
    printf "${DIM}  Remove AIRchetipo skills from your tools${RESET}\n"
    echo ""
    echo ""

    # Tool selection
    printf "${BOLD}  Select tools to clean up:${RESET}\n\n"
    interactive_menu

    if [[ ${#SELECTED_TOOLS[@]} -eq 0 ]]; then
      printf "${YELLOW}  No tools selected. Exiting.${RESET}\n"
      exit 0
    fi

    # Cleanup
    printf "${BOLD}  Cleaning up...${RESET}\n"
    for tool_index in "${SELECTED_TOOLS[@]}"; do
      cleanup_for_tool "$tool_index"
    done

    # Summary
    echo ""
    printf "${GREEN}${BOLD}  Done!${RESET} Cleaned up %d tool(s).\n" "${#SELECTED_TOOLS[@]}"
    echo ""
    printf "${DIM}  Press Enter to exit...${RESET}"
    read -r < /dev/tty
    return
  fi

  printf "${DIM}  Install AIRchetipo skills for your tools${RESET}\n"
  echo ""

  local source_dir=""

  if [[ $use_local -eq 1 ]]; then
    # Use local ./skills/ folder
    if [[ ! -d "./skills" ]]; then
      printf "${RED}Error: ./skills/ folder not found. Run from the repository root.${RESET}\n"
      exit 1
    fi
    source_dir="./skills"
    printf "${DIM}  Using local skills folder...${RESET}\n"
  else
    # Check for curl or wget
    local downloader=""
    if command -v curl &>/dev/null; then
      downloader="curl"
    elif command -v wget &>/dev/null; then
      downloader="wget"
    else
      printf "${RED}Error: curl or wget is required but neither was found.${RESET}\n"
      exit 1
    fi

    # Create temp directory
    TMPDIR_INSTALL=$(mktemp -d)

    # Download zip archive
    printf "${DIM}  Downloading skills...${RESET}\n"
    local zip_file="$TMPDIR_INSTALL/airchetipo.zip"

    if [[ "$downloader" == "curl" ]]; then
      if ! curl -fsSL "$REPO_ZIP" -o "$zip_file" 2>/dev/null; then
        printf "  ${RED}✗${RESET} Failed to download repository\n"
        exit 1
      fi
    else
      if ! wget -q "$REPO_ZIP" -O "$zip_file" 2>/dev/null; then
        printf "  ${RED}✗${RESET} Failed to download repository\n"
        exit 1
      fi
    fi

    # Extract zip
    if ! unzip -q "$zip_file" -d "$TMPDIR_INSTALL" 2>/dev/null; then
      printf "  ${RED}✗${RESET} Failed to extract archive\n"
      exit 1
    fi

    source_dir="$TMPDIR_INSTALL/AIRchetipo-main/skills"
    printf "  ${GREEN}✓${RESET} Downloaded skills\n"
  fi

  echo ""

  # Tool selection
  printf "${BOLD}  Select tools to install for:${RESET}\n\n"
  interactive_menu

  if [[ ${#SELECTED_TOOLS[@]} -eq 0 ]]; then
    printf "${YELLOW}  No tools selected. Exiting.${RESET}\n"
    exit 0
  fi

  # Backend selection
  printf "${BOLD}  Select backend:${RESET}\n\n"
  select_backend

  # Install
  printf "${BOLD}  Installing...${RESET}\n"
  for tool_index in "${SELECTED_TOOLS[@]}"; do
    install_for_tool "$tool_index" "$source_dir"
  done

  # Install config
  install_config "$source_dir" "$SELECTED_BACKEND"

  # Summary
  echo ""
  printf "${GREEN}${BOLD}  Done!${RESET} Installed %d skill(s) for %d tool(s).\n" "${#SKILL_NAMES[@]}" "${#SELECTED_TOOLS[@]}"
  echo ""
  printf "${DIM}  Press Enter to exit...${RESET}"
  read -r < /dev/tty
}

main "$@"
