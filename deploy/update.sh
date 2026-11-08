#!/bin/sh
# Install a KaraokeNino update from a USB disk (no internet).
#
# Put a folder named "karaokenino-update" at the top of the music disk (or
# any USB stick) with:
#     karaokenino           new program
#     karaokenino.sha256    checksum (made by "make release")
#     catalog.db            optional: a catalog built on another computer
#
# Each update is installed once (its checksum is remembered). The previous
# program is kept as karaokenino.prev for automatic rollback.

ROOT="${KARAOKE_ROOT:-/opt/karaokenino}"
MARK="$ROOT/data/.update-instalado"

find_update() {
	for base in /media /mnt /run/media; do
		[ -d "$base" ] || continue
		find "$base" -maxdepth 4 -type d -iname karaokenino-update 2>/dev/null | while read -r d; do
			if [ -f "$d/karaokenino" ] && [ -f "$d/karaokenino.sha256" ]; then
				echo "$d"
				break
			fi
		done
	done | head -n 1
}

UPD="$(find_update)"
[ -n "$UPD" ] || exit 0

SUM="$(cut -d' ' -f1 "$UPD/karaokenino.sha256")"
[ -f "$MARK" ] && [ "$(cat "$MARK")" = "$SUM" ] && exit 0

echo "$(date '+%Y-%m-%d %H:%M:%S') actualización encontrada en $UPD"
if ! (cd "$UPD" && echo "$SUM  karaokenino" | sha256sum -c - >/dev/null 2>&1); then
	echo "  checksum incorrecto: no se instala (¿copia incompleta?)"
	exit 1
fi

TMP="$ROOT/karaokenino.new"
cp -f "$UPD/karaokenino" "$TMP" && chmod +x "$TMP" || exit 1
if ! "$TMP" -version >/dev/null 2>&1; then
	echo "  el programa nuevo no arranca en esta computadora: no se instala"
	rm -f "$TMP"
	exit 1
fi
[ -x "$ROOT/karaokenino" ] && cp -f "$ROOT/karaokenino" "$ROOT/karaokenino.prev"
mv -f "$TMP" "$ROOT/karaokenino"

if [ -f "$UPD/catalog.db" ]; then
	cp -f "$ROOT/data/catalog.db" "$ROOT/data/catalog.db.prev" 2>/dev/null
	cp -f "$UPD/catalog.db" "$ROOT/data/catalog.db"
fi
for s in kiosk-start.sh update.sh; do
	[ -f "$UPD/$s" ] && cp -f "$UPD/$s" "$ROOT/$s" && chmod +x "$ROOT/$s"
done

mkdir -p "$ROOT/data"
echo "$SUM" >"$MARK"
echo "  instalada versión $("$ROOT/karaokenino" -version 2>/dev/null)"
