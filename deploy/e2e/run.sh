#!/bin/sh
# End-to-end test on Linux (run by "make test-e2e" in a privileged container):
#   - the music disk is an UNMOUNTED FAT partition (like Puppy) -> auto-mount;
#   - empty catalog -> built by the disk scan;
#   - a real video plays INSIDE KaraokeNino (libmpv): fullscreen, preview box,
#     dialogs drawn on top, no separate mpv window.
# Screenshots go to /out (deploy/e2e/out). Exits non-zero if a check fails.
set -e
apt-get update -qq >/dev/null
apt-get install -y -qq --no-install-recommends mpv libmpv2 ffmpeg xvfb xdotool imagemagick x11-utils x11-apps dosfstools mtools libgl1-mesa-dri fonts-dejavu-core >/dev/null 2>&1
cd /src && go build -o /tmp/kn ./cmd/karaokenino
# --- music disk as an unmounted FAT partition (like Puppy) ---
mkdir -p /lib/master.1000/karaokes/"Prueba Artista" /lib/master.1000/musica/"Otro Cantante"
ffmpeg -loglevel error -f lavfi -i "testsrc2=size=640x360:rate=25:duration=12" -f lavfi -i "sine=frequency=440:duration=12" \
  -vf "drawtext=text='VIDEO DE PRUEBA %{pts\:hms}':fontsize=36:fontcolor=white:x=40:y=40:fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf" \
  -c:v mpeg4 -c:a mp3 -shortest "/lib/master.1000/karaokes/Prueba Artista/Cancion 1.AVI"
# Second karaoke, SAME size: after the first ends it must be seen, not only heard.
ffmpeg -loglevel error -f lavfi -i "smptebars=size=640x360:rate=25:duration=30" -f lavfi -i "sine=frequency=660:duration=30" \
  -vf "drawtext=text='SEGUNDO VIDEO %{pts\:hms}':fontsize=40:fontcolor=white:x=40:y=280:fontfile=/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf" \
  -c:v mpeg4 -c:a mp3 -shortest "/lib/master.1000/karaokes/Prueba Artista/Cancion 2.avi"
ffmpeg -loglevel error -f lavfi -i "sine=frequency=330:duration=20" "/lib/master.1000/musica/Otro Cantante/Tema.mp3"
# Cover with Windows-style upper-case name (Linux is case-sensitive).
convert -size 320x320 xc:orange -fill white -font /usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf -pointsize 70 -annotate +40+190 'FOTO' "/lib/master.1000/karaokes/Prueba Artista/IMAGES.JPG"
truncate -s 64M /disk.img && mkfs.vfat -n MUSICA /disk.img >/dev/null
mcopy -s -i /disk.img /lib/master.1000 ::/
LOOP=$(losetup -f --show /disk.img); echo "disco: $LOOP sin montar"; blkid $LOOP
# --- config: real playback, auto-mount, empty catalog ---
mkdir -p /run/kn && cat > /run/kn/config.yaml <<CFG
fullscreen: true
demo_mode: false
catalog_db: "/run/kn/catalog.db"
state_file: "/run/kn/state.json"
log_file: "/run/kn/kn.log"
library: { roots: [], dir_name: "master.1000", search_in: ["/mnt"], auto_mount: true, wait_seconds: 20 }
playback: { background_music: { enabled: false }, normalize_audio: true }
mpv_args: ["--ao=null", "--msg-level=all=v", "--log-file=/run/kn/mpv.log"]
idle: { after_seconds: 0 }
CFG
Xvfb :99 -screen 0 1280x720x24 >/dev/null 2>&1 & sleep 2
export DISPLAY=:99 LIBGL_ALWAYS_SOFTWARE=1
/tmp/kn -config /run/kn/config.yaml >/dev/null 2>&1 &
# Wait for real events in the log instead of fixed sleeps.
wait_log() { n=0; until grep -q "$1" /run/kn/kn.log 2>/dev/null; do n=$((n+1)); [ $n -gt ${2:-40} ] && { echo "  (no llegó: $1)"; return 1; }; sleep 0.5; done; }
shot() { xwd -root -silent | convert xwd:- "/out/$1.png"; }
wait_log "escaneo:"; sleep 3                 # catalog built, mode screen up
W=$(xdotool search --name KaraokeNino | head -1)
xdotool windowfocus --sync $W 2>/dev/null || true
shot e2e-0-modos
xdotool key --window $W Return; sleep 1      # mode: Karaoke
xdotool key --window $W Return Return; sleep 1   # letters -> artists -> songs
shot e2e-1b-navegacion
xdotool key --window $W Return               # queue "Cancion 1" (12 s)
wait_log "empieza «Cancion 1"; wait_log "primer cuadro"; sleep 2
shot e2e-2-video-full
xdotool key --window $W 2; sleep 1           # leave fullscreen (consumed)
shot e2e-3-video-recuadro
xdotool key --window $W 0; sleep 1
shot e2e-4-dialogo
xdotool key --window $W Escape; sleep 1
shot e2e-5-cerrado
xdotool key --window $W 2 Return; sleep 1    # select and queue "Cancion 2"
xdotool key --window $W 7                    # fullscreen; wait for "Cancion 1" to end
wait_log "empieza «Cancion 2" 60; sleep 2
shot e2e-6-segundo-video
echo "--- arbol:"; xwininfo -root -tree | grep -E "x[0-9a-f]+ " | head -12
xwd -root -silent | convert xwd:- /out/e2e-2-video-full.png
xdotool key --window $W 2; sleep 2
xwd -root -silent | convert xwd:- /out/e2e-3-video-recuadro.png
xdotool key --window $W 0; sleep 1
xwd -root -silent | convert xwd:- /out/e2e-4-dialogo.png
xdotool key --window $W Escape; sleep 1
xwd -root -silent | convert xwd:- /out/e2e-5-cerrado.png
echo "--- montajes:"; grep loop /proc/mounts || true
echo "--- log:"; grep -E "reproductor|DEBUG" /run/kn/kn.log; echo "--- duracion:"; ffprobe -v error -show_entries format=duration -of csv=p=0 "/lib/master.1000/karaokes/Prueba Artista/Cancion 1.AVI"; echo "--- mpv fin:"; grep -iE "end-file|EOF reached|finished playback|reason|Terminating|error" /run/kn/mpv.log | grep -v "Setting option\|Command line" | head -12 ; echo "--- mpv.log:"; grep -iE "parent|embed|wid|invalid|geometry|window size|resize|reconfig" /run/kn/mpv.log | grep -v "Command line\|Setting option" | head -20

# --- checks ---
fail=0
check() { if eval "$2"; then echo "OK   $1"; else echo "FALLA $1"; fail=1; fi; }
check "montó el disco solo y de lectura" "grep -q 'master.1000' /run/kn/kn.log && grep -q ' /mnt/loop[0-9]* vfat ro' /proc/mounts"
check "el escaneo armó el catálogo" "grep -q 'escaneo: 3 archivos, 3 nuevas' /run/kn/kn.log"
check "miniatura de la portada (IMAGES.JPG)" "ls /run/kn/thumbs/*.jpg >/dev/null 2>&1"
check "el segundo video se ve (no solo se oye)" "convert /out/e2e-6-segundo-video.png -crop 200x200+540+260 -format '%[fx:standard_deviation]' info: | awk '{exit !(\$1 > 0.05)}'"
check "empezaron los dos videos" "grep -q 'empieza «Cancion 1' /run/kn/kn.log && grep -q 'empieza «Cancion 2' /run/kn/kn.log"
check "el registro confirma cuadros de los dos videos" "[ \$(grep -c 'primer cuadro del video' /run/kn/kn.log) -ge 2 ]"
check "reproductor integrado (libmpv)" "grep -q 'reproductor integrado (libmpv)' /run/kn/kn.log"
check "sin ventana aparte de mpv" "! pgrep -x mpv >/dev/null && [ \$(xwininfo -root -children | grep -c '\"') -le 2 ]"
exit $fail
