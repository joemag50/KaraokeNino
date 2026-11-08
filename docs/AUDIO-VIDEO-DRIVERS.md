# Audio, video y drivers (Debian / Puppy, sin internet)

KaraokeNino **no usa** DirectShow, FFDShow ni Xvid de Windows. En Linux el stack es distinto y, en general, **más simple** de mantener offline.

## ¿Necesitamos drivers “extra”?

| Componente | ¿Driver especial? | Qué usar |
|------------|-------------------|----------|
| **Audio** | Casi nunca | **ALSA** (viene con el kernel). En Debian/Puppy suele bastar. Si hay mezclador, **PipeWire** o **PulseAudio** (Debian 12 suele traer PipeWire). |
| **Video (pantalla)** | No para 2D | Driver **modesetting** / GPU del kernel (Intel/AMD/NVIDIA legacy). Solo instala firmware si el live USB no da imagen. |
| **Reproducción** | No codecs sueltos tipo K-Lite | **`mpv`** (+ **ffmpeg** libs que trae el paquete `mpv`). |
| **Karaoke .avi/.mp4/.mkv** | No | **mpv** |
| **Karaoke .cdg** | No | mpv reproduce el `.cdg` junto con el `.mp3` del mismo nombre (KaraokeNino lo pasa solo). `.kar` (MIDI) no está soportado. |

## Paquetes offline recomendados (Debian amd64)

Descarga en la PC **con internet** (misma versión que la rockola) y lleva en USB:

```text
mpv
libmpv2             # video integrado dentro de KaraokeNino
ffmpeg (dependencia de mpv, apt lo resuelve)
alsa-utils          # opcional: aplay, alsamixer
```

Comprobación en la rockola **sin red**:

```bash
aplay -l                 # lista dispositivos de audio
mpv --version
mpv --fullscreen /ruta/prueba.mp4
```

## Audio: volumen y salida

- Ajuste persistente: `alsamixer` o panel de Puppy.
- KaraokeNino controla el volumen de **mpv** por IPC (teclas +/−) con el tope `playback.max_volume`.
- `playback.normalize_audio` iguala el volumen entre canciones (filtro `dynaudnorm`, ligero).
- Salida HDMI u otra tarjeta: `mpv_args: ["--audio-device=alsa/hdmi"]` (lista con `mpv --audio-device=help`).
- Si no hay sonido: cable/salida correcta, mute en ALSA, dispositivo por defecto.

## Video: pantalla completa

- La interfaz es una ventana nativa (Fyne) en pantalla completa; no usa navegador.
- El video se reproduce **dentro de KaraokeNino** con **libmpv** (paquete `libmpv2`):
  mpv decodifica video y audio y entrega cada cuadro como imagen; la pantalla lo
  dibuja a pantalla completa o en el recuadro de vista previa. No hay ventana aparte,
  el teclado siempre llega a KaraokeNino y los avisos ("Siguiente…", teclas) se dibujan
  encima del video. El cuadro se calcula al tamaño del video (los karaokes son SD) y la
  tarjeta de video lo agranda, así que no carga el procesador.
- Sin `libmpv2` (o con `playback.embedded_video: false`) se usa mpv como programa
  aparte en pantalla completa, como respaldo. Admin → Diagnóstico dice cuál se usa.
- Las canciones solo de audio no abren ventana de mpv: se queda la pantalla con la portada.
- Video lento en máquinas viejas: prueba `mpv_args: ["--hwdec=auto"]`.

## Puppy Linux

Depende de la variante (Bookworm, etc.). Opciones:

1. `.deb` de Debian compatible + `dpkg -i` (y deps en la misma carpeta).
2. Pet/SFS con mpv preinstalado en tu imagen Puppy.

**No** hace falta instalar drivers de Windows ni K-Lite Codec Pack.

## Resumen

- **Sí necesitas:** kernel + ALSA + X11 + **mpv** + **libmpv2**.
- **No necesitas:** FFDShow, Xvid standalone, KaraokeForDirectX (Windows).
- **2 GB RAM:** suficiente para mpv + KaraokeNino.
