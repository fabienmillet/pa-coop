#!/bin/bash
# Construit les paquets pa-coop dans dist/ :
#   pa-coop-windows.zip      : dbghelp.dll + pacoop-server.exe + pacoop.txt + LISEZMOI.txt
#   pa-coop-linux.tar.gz     : pacoop.so + pacoop-server + pacoop.txt + LISEZMOI.txt
#   pacoop-server-linux-amd64: serveur dédié seul
set -euo pipefail
cd "$(dirname "$0")"
OUT=dist
rm -rf "$OUT" && mkdir -p "$OUT/windows" "$OUT/linux"

( cd server && go vet -composites=false ./... \
  && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "../$OUT/windows/pacoop-server.exe" ./cmd/pacoop-server \
  && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "../$OUT/linux/pacoop-server" ./cmd/pacoop-server )
cp "$OUT/linux/pacoop-server" "$OUT/pacoop-server-linux-amd64"

x86_64-w64-mingw32-gcc -O2 -Wall -shared -s -o "$OUT/windows/dbghelp.dll" client/win/pacoop_dbghelp.c client/win/pacoop_resync.c client/win/dbghelp.def -static-libgcc
gcc -O2 -Wall -shared -fPIC -o "$OUT/linux/pacoop.so" client/linux/pacoop_preload.c client/linux/pacoop_resync.c -ldl -lpthread

for d in windows linux; do
  cp docs/pacoop.txt "$OUT/$d/pacoop.txt"
  cp "docs/LISEZMOI-$d.txt" "$OUT/$d/LISEZMOI.txt"
done
( cd "$OUT/windows" && zip -q -9 ../pa-coop-windows.zip * )
( cd "$OUT/linux" && tar czf ../pa-coop-linux.tar.gz * )
ls -la "$OUT"
