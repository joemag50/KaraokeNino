# Velocidad y rendimiento

Leer `Musica.data` (~175k líneas) en cada arranque sería lento. KaraokeNino lo
indexa una vez en **SQLite** y consulta en caliente.

| Qué | Cómo |
|-----|------|
| Importar Musica.data completo (165,790 canciones) | ~10 s, una transacción (incluye índice de búsqueda) |
| Arranque diario | Solo abre `catalog.db` (queda guardado para siempre) y lee la página visible; el disco no se recorre (eso es desde el admin) |
| Escaneo incremental | Solo escribe archivos nuevos/faltantes; compara sin mayúsculas |
| Búsqueda por texto | SQLite **FTS5** con prefijos y sin acentos |
| Páginas de artistas | Conteo de canciones guardado por artista: contar 2,697 artistas bajó de 68 ms a 0.5 ms |
| Arranque | El registro dice cuánto tardó (`pantalla lista … en X`, `disco … encontrado en X`) |
| Portadas | Miniaturas de 240 px en `data/thumbs`, hechas una sola vez en segundo plano; la pantalla nunca lee el disco de música |
| Música de fondo | Elige al azar por id, sin `ORDER BY RANDOM()` sobre 166k filas |
| Interfaz | Primitivas de canvas escaladas; sin navegador. Fuentes incluidas en el programa (Nunito y Fredoka, ~75 KB) |
| Animaciones | Fondo de fiesta a 10 cuadros/s: luces hechas una sola vez (imágenes de 64 px que escala la tarjeta de video) y círculos que solo se mueven; nada se redibuja en el procesador. Se pausan solas mientras suena un video y bajo la pantalla de espera. Un cuadro cuesta ~4 µs de nuestro código (`go test -bench PartyFrame ./internal/ui`); lo demás es que Fyne vuelve a pintar la ventana. En una PC muy lenta: `ui.animations: false` |
| Audio | `dynaudnorm` (ligero) en lugar de `loudnorm` |

## Hardware (2 GB RAM)

- Preferir **SSD o USB 3** para la biblioteca de video.
- Video lento: `mpv_args: ["--hwdec=auto"]`.
- La skin `moderno` se dibuja a cualquier resolución; en TVs 4K conviene poner el
  escritorio a 1080p.

## Discos de varios TB (2026-11)

Medido en una PC rápida (los números de la rockola serán varias veces más
lentos, pero las proporciones se mantienen). Pruebas: `go test -bench . ./internal/catalog`
y la prueba de escala `KN_SCALE_N=1000000 go test -run TestScanAtScale -v ./internal/catalog`.

| Qué | Antes | Ahora |
|-----|------:|------:|
| Escaneo de 1 millón de archivos, catálogo vacío: memoria máxima | 550 MB | **41 MB** |
| Re-escaneo de 1 millón de archivos ya conocidos: memoria máxima | 551 MB | **181 MB** (86 MB vivos) |
| Re-escaneo de 1 millón (sin pausas) | ~14 s | **10 s** |
| Agregar una canción (escaneo, importación) | 53 µs | **35 µs** (consultas preparadas) |
| Búsqueda de 1 letra («a»), catálogo real | 128 ms | **3 ms** |
| Búsqueda de 2 letras («la»), catálogo real | 52 ms | **≤ 12 ms** |
| Canción al azar de MUSICA (música de fondo), 100 mil canciones | 6.5 ms | **0.03 ms** |
| Diagnóstico (conteos y duplicados), catálogo real | 319 ms | **39 ms** |
| Abrir un archivo cuyo género tiene otras mayúsculas (`musica` vs `MUSICA`, 6,000 artistas), Linux | 1.6 ms y 830 KB por vez | **0.011 ms** |

Cómo:

- **Escaneo en lotes**: escribe de 2,000 en 2,000 canciones (antes, todo en una
  sola transacción al final): la memoria no crece con el disco, las canciones
  nuevas aparecen mientras sigue, las reproducciones se registran entre lotes
  (antes una transacción de minutos podía hacerlas fallar), el archivo WAL no
  crece al tamaño del catálogo y un escaneo interrumpido conserva lo hecho.
- **Lo conocido se guarda como hashes de 64 bits**, no como rutas, y lo visto
  como un bit por canción.
- **No lee archivos ni hace `stat`**: una lectura de carpeta por carpeta (los
  tipos vienen en la lista). No se usa la fecha de las carpetas para saltarlas
  porque en FAT/exFAT no cambia de forma confiable al copiar canciones.
- **No compite con el video**: entre carpeta y carpeta deja descansar el disco
  (la mitad del tiempo; dos tercios si `ScanOptions.Busy` dice que algo suena);
  en Linux el hilo del escaneo tiene prioridad baja de CPU y de disco.
- **Disco desconectado a medias**: si alguna carpeta no se pudo leer, el escaneo
  agrega lo nuevo pero no oculta nada (antes ocultaba todo lo que faltaba por leer).
- **Índice de búsqueda**: se agrega canción por canción (antes se rehacía
  completo, ~0.7 s, en cada escaneo con cambios); índices de prefijos de 2 y 3
  letras; con palabras de 1–2 letras se devuelven las primeras coincidencias
  sin ordenar por relevancia (ordenar 80 mil resultados para mostrar 60 era lo lento).
- **SQLite** (`internal/catalog/store.go`): WAL con `synchronous=NORMAL`,
  caché de 16 MB por conexión, `mmap` de 128 MB, temporales en memoria,
  WAL limitado a 64 MB; índice cubriente para duplicados; `PRAGMA optimize`
  al cerrar y después de cada escaneo.
- **Reproducir del USB lento**: mpv lee por adelantado hasta 48 MB
  (`--cache=yes --demuxer-max-bytes=48MiB`); antes, con archivos locales, solo
  ~1 s, y cualquier pausa del disco trababa el video. Se puede cambiar en `mpv_args`.
- **Mayúsculas de Windows**: la lista de las carpetas grandes (géneros) se
  recuerda; antes cada canción y cada portada volvía a leer la carpeta del género.
- **Registros**: `kiosk.log` ya no recibe una copia de cada línea del registro
  y rota a 5 MB; `crash.log` tiene tope de 1 MB.

Mantenimiento periódico: [MANTENIMIENTO.md](MANTENIMIENTO.md).
