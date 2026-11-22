#!/bin/bash
# pa-coop : installation du mode multijoueur pour Prison Architect (Linux, version Steam native)
#   curl -fsSL https://raw.githubusercontent.com/fabienmillet/prison-architect-server/master/install/install.sh | bash
set -euo pipefail
REPO=fabienmillet/prison-architect-server
FILES="pacoop.so pacoop-server pacoop.txt"

say() { printf '%s\n' "$*"; }
ask() { local r; read -r -p "$1" r < /dev/tty; printf '%s' "$r"; }

# 1. Trouver le jeu (Steam natif ou flatpak, toutes les bibliothèques)
games=()
for root in "$HOME/.steam/steam" "$HOME/.local/share/Steam" "$HOME/.var/app/com.valvesoftware.Steam/.local/share/Steam"; do
  vdf="$root/steamapps/libraryfolders.vdf"
  libs=("$root")
  [ -f "$vdf" ] && while IFS= read -r l; do libs+=("$l"); done < <(sed -n 's/.*"path"[[:space:]]*"\(.*\)".*/\1/p' "$vdf")
  for l in "${libs[@]}"; do
    d="$l/steamapps/common/Prison Architect"
    [ -x "$d/PrisonArchitect.x86_64" ] && games+=("$(readlink -f "$d")")
  done
done
mapfile -t games < <(printf '%s\n' "${games[@]}" | awk 'NF && !seen[$0]++')

say "=== pa-coop : multijoueur Prison Architect sur ton propre serveur ==="
if [ ${#games[@]} -eq 0 ]; then
  game=$(ask 'Prison Architect introuvable. Chemin du dossier du jeu : ')
  [ -x "$game/PrisonArchitect.x86_64" ] || { say "Dossier invalide."; exit 1; }
elif [ ${#games[@]} -eq 1 ]; then
  game=${games[0]}
else
  for i in "${!games[@]}"; do say "  $((i + 1))) ${games[$i]}"; done
  n=$(ask 'Quelle installation ? ')
  game=${games[$((n - 1))]}
fi
say "Jeu : $game"

say ""
say "Que veux-tu faire ?"
say "  1) Héberger une partie (tes amis te rejoignent)"
say "  2) Rejoindre un ami ou un serveur"
say "  3) Revenir au multijoueur officiel (désinstaller)"
mode=$(ask 'Ton choix : ')

if [ "$mode" = 3 ]; then
  for f in $FILES pacoop-server.log pacoop-selecteur.log pa-coop-LISEZMOI.txt; do rm -f "$game/$f"; done
  say "pa-coop désinstallé. Pense à retirer LD_PRELOAD des options de lancement Steam du jeu."
  exit 0
fi

# 2. Télécharger et installer
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
say "Téléchargement de la dernière version de pa-coop..."
if [ -n "${PACOOP_TARBALL:-}" ]; then cp "$PACOOP_TARBALL" "$tmp/pa-coop.tar.gz"   # test : paquet local
else curl -fsSL -o "$tmp/pa-coop.tar.gz" "https://github.com/$REPO/releases/latest/download/pa-coop-linux.tar.gz"; fi
tar -xzf "$tmp/pa-coop.tar.gz" -C "$tmp"
install -m 755 "$tmp/pacoop.so" "$tmp/pacoop-server" "$game/"
install -m 644 "$tmp/LISEZMOI.txt" "$game/pa-coop-LISEZMOI.txt"

# 3. Configuration
if [ "$mode" = 1 ]; then
  printf '# pa-coop : héberger (le jeu lance le serveur)\nserver=local\nupnp=1\n' > "$game/pacoop.txt"
  say ""
  say "C'est prêt ! Charge une prison puis Menu > « Go Online »."
  ip=$(curl -fsS -m 5 https://api.ipify.org 2>/dev/null || true)
  [ -n "$ip" ] && say "Tes amis mettront ton adresse : $ip (port 4533, ouvert par UPnP si ta box le permet)"
else
  addr=$(ask "Adresse de l'hôte ou du serveur (ex. 82.64.12.34 ou pa.monserveur.fr:4533) : ")
  [ -n "$addr" ] || { say "Adresse vide : annulé."; exit 1; }
  printf '# pa-coop : rejoindre\nserver=%s\n' "$addr" > "$game/pacoop.txt"
  say ""
  say "C'est prêt ! Menu > « Join Online Game » : la partie de ton ami apparaît dans la liste."
fi
say ""
say "Dernière étape (une seule fois) : dans Steam, Prison Architect > Propriétés > Options de lancement :"
say '    LD_PRELOAD="./pacoop.so" %command%'
