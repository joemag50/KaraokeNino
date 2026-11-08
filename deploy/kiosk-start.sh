#!/bin/sh
# KaraokeNino kiosk loop (no internet needed).
#
#  - installs an update from the USB disk if there is one (update.sh);
#  - starts KaraokeNino fullscreen and starts it again if it closes or crashes;
#  - if a new version crashes 3 times in a row right after starting, it goes
#    back to the previous version;
#  - keeps the screen from blanking and hides the mouse pointer.
#
# Exit codes from karaokenino: 10 = admin chose "Salir al escritorio" (stop),
# anything else = start again.

ROOT="${KARAOKE_ROOT:-/opt/karaokenino}"
CONFIG="${KARAOKE_CONFIG:-$ROOT/config/config.yaml}"
LOG="$ROOT/data/logs/kiosk.log"

mkdir -p "$ROOT/data/logs"
cd "$ROOT" || exit 1

log() { echo "$(date '+%Y-%m-%d %H:%M:%S') $*" >>"$LOG"; }

# Only one kiosk loop at a time (autostart + systemd + .xinitrc overlap).
LOCK="/tmp/karaokenino-kiosk.lock"
if [ -e "$LOCK" ] && kill -0 "$(cat "$LOCK" 2>/dev/null)" 2>/dev/null; then
	exit 0
fi
echo $$ >"$LOCK"
trap 'rm -f "$LOCK"' EXIT

if [ -n "$DISPLAY" ]; then
	xset s off -dpms s noblank 2>/dev/null
	command -v unclutter >/dev/null 2>&1 && unclutter -idle 2 -root >/dev/null 2>&1 &
fi

fails=0
while true; do
	# kiosk.log only grows (everything karaokenino and update.sh print):
	# keep it and one older copy under ~5 MB each.
	if [ "$(wc -c <"$LOG" 2>/dev/null || echo 0)" -gt 5242880 ]; then
		mv -f "$LOG" "$LOG.1"
	fi
	"$ROOT/update.sh" >>"$LOG" 2>&1

	start=$(date +%s)
	"$ROOT/karaokenino" -config "$CONFIG" >>"$LOG" 2>&1
	code=$?
	took=$(( $(date +%s) - start ))
	log "karaokenino terminó (código $code, ${took}s)"

	[ "$code" -eq 10 ] && { log "salida al escritorio"; exit 0; }

	if [ "$code" -ne 0 ] && [ "$code" -ne 3 ] && [ "$took" -lt 20 ]; then
		fails=$((fails + 1))
	else
		fails=0
	fi
	if [ "$fails" -ge 3 ] && [ -x "$ROOT/karaokenino.prev" ]; then
		log "la versión nueva falla al arrancar: se regresa a la anterior"
		mv -f "$ROOT/karaokenino" "$ROOT/karaokenino.fallida"
		mv -f "$ROOT/karaokenino.prev" "$ROOT/karaokenino"
		fails=0
	fi
	sleep 2
done
