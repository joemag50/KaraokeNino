#!/bin/sh
# Install KaraokeNino on the rockola (Debian or Puppy Linux), offline.
#
#   sudo ./install.sh                 # desktop autostart for the current user
#   sudo ./install.sh --kiosk USER    # no desktop: autologin on tty1 + X only for KaraokeNino
#
# Run it from the unpacked release folder. Re-running it upgrades the program
# and keeps config.yaml, the catalog and the play history.

set -e
SRC="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="${KARAOKE_ROOT:-/opt/karaokenino}"
MODE=desktop
KUSER="${SUDO_USER:-$(id -un)}"
[ "$1" = "--kiosk" ] && { MODE=kiosk; KUSER="${2:-$KUSER}"; }

if [ "$(id -u)" -ne 0 ]; then
	echo "Ejecuta con sudo (o como root en Puppy)." >&2
	exit 1
fi

echo "Instalando en $ROOT…"
mkdir -p "$ROOT/config" "$ROOT/data/logs"
install -m 755 "$SRC/karaokenino" "$ROOT/karaokenino"
[ -f "$SRC/importcatalog" ] && install -m 755 "$SRC/importcatalog" "$ROOT/importcatalog"
install -m 755 "$SRC/deploy/kiosk-start.sh" "$ROOT/kiosk-start.sh"
install -m 755 "$SRC/deploy/update.sh" "$ROOT/update.sh"
cp -f "$SRC/config/config.example.yaml" "$ROOT/config/config.example.yaml"
if [ ! -f "$ROOT/config/config.yaml" ]; then
	cp "$SRC/config/config.example.yaml" "$ROOT/config/config.yaml"
	echo "  creado config/config.yaml — CAMBIA admin.pin antes de rentar"
fi
[ -f "$SRC/data/catalog.db" ] && [ ! -f "$ROOT/data/catalog.db" ] && cp "$SRC/data/catalog.db" "$ROOT/data/"

if id "$KUSER" >/dev/null 2>&1; then
	chown -R "$KUSER" "$ROOT/data" "$ROOT/config"
fi
command -v mpv >/dev/null 2>&1 || echo "  AVISO: falta mpv (instala el .deb de mpv sin internet; ver docs/AUDIO-VIDEO-DRIVERS.md)"
if ! ldconfig -p 2>/dev/null | grep -q "libmpv\.so"; then
	echo "  AVISO: falta libmpv2: sin ella el video sale en una ventana aparte de mpv."
	echo "         Instálala con: apt install libmpv2   (o el .deb sin internet)"
fi

# ---- autostart ----
if [ -f /etc/DISTRO_SPECS ]; then
	# Puppy Linux: everything in /root/Startup runs after the desktop starts.
	mkdir -p /root/Startup
	ln -sf "$ROOT/kiosk-start.sh" /root/Startup/karaokenino
	sed -i 's/^\(\s*shutdown_command:\).*/\1 ["poweroff"]/; s/^\(\s*reboot_command:\).*/\1 ["reboot"]/' "$ROOT/config/config.yaml"
	echo "Puppy: arranque automático en /root/Startup/karaokenino"
elif [ "$MODE" = kiosk ]; then
	HOME_DIR="$(getent passwd "$KUSER" | cut -d: -f6)"
	# 1. Autologin on tty1.
	mkdir -p /etc/systemd/system/getty@tty1.service.d
	cat >/etc/systemd/system/getty@tty1.service.d/autologin.conf <<EOF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin $KUSER --noclear %I \$TERM
EOF
	# 2. Start X with only KaraokeNino (no desktop to escape to).
	cat >"$HOME_DIR/.xinitrc" <<EOF
#!/bin/sh
exec $ROOT/kiosk-start.sh
EOF
	grep -q "karaokenino-startx" "$HOME_DIR/.bash_profile" 2>/dev/null || cat >>"$HOME_DIR/.bash_profile" <<'EOF'
# karaokenino-startx: start the kiosk on the first console
if [ -z "$DISPLAY" ] && [ "$(tty)" = /dev/tty1 ]; then exec startx -- -nocursor; fi
EOF
	chown "$KUSER" "$HOME_DIR/.xinitrc" "$HOME_DIR/.bash_profile"
	chmod +x "$HOME_DIR/.xinitrc"
	command -v startx >/dev/null 2>&1 || echo "  AVISO: falta xinit (apt install xinit xserver-xorg)"
	# 3. Let the kiosk user power off/reboot from the admin menu.
	echo "$KUSER ALL=(root) NOPASSWD: /usr/bin/systemctl poweroff, /usr/bin/systemctl reboot" >/etc/sudoers.d/karaokenino
	chmod 440 /etc/sudoers.d/karaokenino
	sed -i 's/^\(\s*shutdown_command:\).*/\1 ["sudo", "systemctl", "poweroff"]/; s/^\(\s*reboot_command:\).*/\1 ["sudo", "systemctl", "reboot"]/' "$ROOT/config/config.yaml"
	systemctl daemon-reload || true
	echo "Kiosco: el usuario $KUSER entra solo en tty1 y arranca KaraokeNino sin escritorio"
else
	HOME_DIR="$(getent passwd "$KUSER" | cut -d: -f6)"
	mkdir -p "$HOME_DIR/.config/autostart"
	sed "s#/opt/karaokenino#$ROOT#g" "$SRC/deploy/karaokenino.desktop" >"$HOME_DIR/.config/autostart/karaokenino.desktop"
	chown -R "$KUSER" "$HOME_DIR/.config/autostart"
	echo "Escritorio: KaraokeNino arranca al iniciar sesión de $KUSER"
	echo "  Activa el inicio de sesión automático en tu escritorio (LightDM: autologin-user=$KUSER)"
fi
echo "Listo. Reinicia la computadora para probar."
