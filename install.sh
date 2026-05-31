#!/usr/bin/env bash
# Instalador do vbrain. Clona o projeto, roda ./install.sh, e pronto:
#   1. builda o binário `vbrain` e instala num dir do PATH;
#   2. instala as skills globalmente (~/.claude/skills);
#   3. bootstrapa a base (~/vbrain): CLAUDE.md, skills, git, rotinas;
#   4. (opcional) configura identidade git e cria o repo no GitHub via PAT.
#
# Pré-requisitos: um shell, git (pra clonar) e — se não houver binário
# pré-compilado — o toolchain Go pra buildar. O `vbrain` em si é autocontido
# (SQLite e git embutidos), então depois do install nada de Ruby/gems.
set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${VBRAIN_BIN_DIR:-$HOME/.local/bin}"
SKILLS_DIR="${VBRAIN_SKILLS_DIR:-$HOME/.claude/skills}"

say() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
ask() { local p="$1" d="${2:-}"; local a; read -r -p "$p" a || true; printf '%s' "${a:-$d}"; }

# --- 1. binário ------------------------------------------------------------
if ! command -v go >/dev/null 2>&1; then
  echo "Go não encontrado. Instale o Go (https://go.dev/dl/) e rode de novo," >&2
  echo "ou coloque um binário 'vbrain' pré-compilado em $BIN_DIR." >&2
  exit 1
fi
say "Buildando o binário vbrain…"
mkdir -p "$BIN_DIR"
( cd "$REPO" && go build -o "$BIN_DIR/vbrain" ./cmd/vbrain )
say "vbrain instalado em $BIN_DIR/vbrain"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "  (adicione $BIN_DIR ao seu PATH: export PATH=\"$BIN_DIR:\$PATH\")" ;;
esac
VBRAIN="$BIN_DIR/vbrain"

# --- 2. skills globais -----------------------------------------------------
say "Instalando skills em $SKILLS_DIR…"
mkdir -p "$SKILLS_DIR"
cp -R "$REPO/.claude/skills/." "$SKILLS_DIR/"

# --- 3. identidade git (se faltar) ----------------------------------------
if command -v git >/dev/null 2>&1; then
  if [ -z "$(git config --global user.name || true)" ]; then
    name="$(ask 'Seu nome para os commits do git: ')"
    [ -n "$name" ] && git config --global user.name "$name"
  fi
  if [ -z "$(git config --global user.email || true)" ]; then
    email="$(ask 'Seu email para os commits do git: ')"
    [ -n "$email" ] && git config --global user.email "$email"
  fi
fi

# --- 4. onboarding GitHub (opcional) --------------------------------------
GH_ARGS=()
echo
say "Versionar a base no GitHub? (recomendado — backup + sync entre máquinas)"
vis="$(ask '  [p]rivado / p[u]blico / [n] enhum (default: privado): ' p)"
case "$vis" in
  u|public)  GH_ARGS=(--github public) ;;
  n|none)    GH_ARGS=() ;;
  *)         GH_ARGS=(--github private) ;;
esac
if [ "${#GH_ARGS[@]}" -gt 0 ]; then
  if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "  Preciso de um Personal Access Token (PAT) com escopo 'repo'."
    echo "  Crie em: https://github.com/settings/tokens/new?scopes=repo&description=vbrain"
    GITHUB_TOKEN="$(ask '  Cole o PAT (deixe vazio pra pular o GitHub): ')"
  fi
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    export GITHUB_TOKEN
    repo="$(ask '  Nome do repo (default: vbrain): ' vbrain)"
    GH_ARGS+=(--repo-name "$repo")
  else
    GH_ARGS=()
  fi
fi

# --- 5. bootstrap da base --------------------------------------------------
echo
say "Bootstrapando a base em ${VBRAIN_HOME:-$HOME/vbrain}…"
"$VBRAIN" setup --skills-src "$REPO/.claude/skills" "${GH_ARGS[@]}"

echo
say "Pronto. Reabra o Claude Code para detectar as skills."
