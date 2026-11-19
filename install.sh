#!/bin/sh
# Dotes installer for macOS and Linux.
#   curl -fsSL https://raw.githubusercontent.com/Tahaldr/Dotes/master/install.sh | sh
# Puts dotes in ~/.local/bin, asks which parts you want, and offers to add the shell function
# to your ~/.bashrc or ~/.zshrc.
set -e

repo=Tahaldr/Dotes
case "$(uname -s)" in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) echo "Dotes: use install.ps1 on Windows"; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "Dotes: no build for $(uname -m)"; exit 1 ;;
esac
dir="$HOME/.local/bin"
mkdir -p "$dir"

printf '\n  \033[36mDotes\033[0m  one terminal app with the parts you pick: Home, Files, Notes, Git\n\n'

# 1. the program
url="https://github.com/$repo/releases/latest/download/dotes_${os}_${arch}"
echo "  Downloading Dotes..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$url" -o "$dir/dotes.download"
else
  wget -qO "$dir/dotes.download" "$url"
fi
chmod +x "$dir/dotes.download"
mv -f "$dir/dotes.download" "$dir/dotes"
echo "  Installed $dir/dotes ($("$dir/dotes" --version))"

case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "  Note: $dir isn't on your PATH yet (the shell function below adds it)"; export PATH="$PATH:$dir" ;;
esac

# 2. the parts (the picker needs the terminal, also when this script comes through a pipe)
printf '\n  Pick the parts you want (space ticks, enter saves). Change it later with: dotes setup\n'
"$dir/dotes" setup </dev/tty >/dev/tty

# 3. the shell function
rc="$HOME/.bashrc"
case "$SHELL" in *zsh) rc="$HOME/.zshrc" ;; esac
if grep -q "dotes shell" "$rc" 2>/dev/null; then
  echo "  $rc already loads Dotes."
else
  printf '\n  Add Dotes to %s? It lets q in Files leave you in that folder,\n  and x on Home close the tab. [Y/n] ' "$rc"
  read -r a </dev/tty || a=n
  case "$a" in
    "" | y* | Y*)
      printf '  Also open Dotes Home in every new terminal tab? [y/N] '
      read -r b </dev/tty || b=n
      start=""
      case "$b" in y* | Y*) start=" --start" ;; esac
      {
        echo ""
        echo "# Dotes (dotes shell bash prints what this loads)"
        echo "export PATH=\"\$PATH:$dir\""
        echo "eval \"\$(command dotes shell bash$start)\""
      } >>"$rc"
      echo "  Added to $rc"
      ;;
  esac
fi

printf '\n  \033[36mDone.\033[0m Open a new terminal and type  dotes  (dotes --help lists everything).\n\n'
