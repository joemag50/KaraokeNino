# KaraokeNino — Requerimientos

Rockola de karaoke y música **en renta** (se lleva a fiestas y eventos). Sin monedas ni créditos.
Hardware objetivo: PC con Debian/Puppy Linux, 2 GB RAM, teclado numérico tipo rockola, sin internet, disco/USB de ~1 TB.

Prioridades: **P0** = MVP para la primera renta · **P1** = v1 · **P2** = después.
Última actualización: 2026-11-02.

---

## 1. Catálogo actual (lo que hay en `Musica.data`)

| Género (carpeta) | Canciones | Raíz original |
|---|---:|---|
| MUSICA | 125,542 | `D:\master.1000\musica` |
| KARAOKES | 26,282 | `D:\master.1000\karaokes` (+ `2026\karaokes 2026`) |
| MUSICA 2026 | 6,219 | `D:\master.1000\2026` |
| VALLENATOS | 3,661 | `D:\master.1000\vallenatos` |
| VIDEOS | 2,298 | `D:\master.1000\videos 80g` |
| z INFANTILES | 1,386 | `D:\master.1000\infantiles` |
| MIXES | 443 | `D:\master.1000\MIXES` |
| **Total** | **165,831** en **9,345** carpetas de artista | |

Formatos: MP3 135,836 · AVI 17,269 · MPG 10,428 · WMV 1,003 · WMA 884 · WAV 165 · MP4 88 · M4A 53 · MPEG 44 · CDG 12 · DAT/FLV/ASF ~9 · sin tipo 40.

Estructura: `Género / Artista / archivo`. 5,654 de 9,345 artistas traen `images.jpg` (portada).
El archivo es texto **ISO-8859-1 (Latin-1)** con fin de línea CRLF:

```
GENERO
\tArtista|carpeta\|carpeta\images.jpg
\t\tcarpeta\|Título (nombre de archivo sin extensión)|TIPO
```

> ✅ **Corregido (2026-11-01):** el importador guardaba la carpeta en lugar del archivo (1 canción por artista) y no convertía Latin-1. Ahora la ruta es `carpeta + nombre + .tipo` (respetando espacios finales como `Tu Y Yo .mpg`), el texto se decodifica como Windows-1252 y todo corre en una transacción. Al reproducir, la ruta se busca sin distinguir mayúsculas (`.AVI` vs `.avi`), porque los nombres vienen de Windows. Se omiten 41 líneas sin tipo de archivo (`importcatalog -v` las muestra).

---

## 2. Requerimientos

Estado: ✅ hecho y probado en pruebas automáticas · 🟡 hecho con una limitación (ver nota) · 🔧 hecho, falta probar en la rockola real (ver sección 3).

### A. Plataforma y arranque
| # | Requerimiento | Prioridad | Estado |
|---|---|---|---|
| A1 | Corre en Debian/Puppy Linux con 2 GB RAM, sin internet | P0 | ✅ binario compilado sobre Debian 12 (glibc 2.36); probado instalando en Debian 12 limpio |
| A2 | Arranca sola al encender la PC, directo a pantalla completa (modo kiosco, sin escritorio visible) | P0 | 🔧 `install.sh`: escritorio, kiosco (autologin tty1 + `startx` sin escritorio) o Puppy (`/root/Startup`) |
| A3 | Si el programa se cierra o falla, se reinicia solo | P0 | ✅ ciclo en `kiosk-start.sh` (probado en contenedor) |
| A4 | Apagado seguro desde el menú admin | P1 | ✅ apagar / reiniciar con confirmación; guarda volumen y ajustes antes |
| A5 | Detecta el disco aunque cambie el punto de montaje | P0 | ✅ `library.roots` + busca `master.1000` bajo `/media`, `/mnt`, `/run/media`; espera 60 s y reintenta cada 10 s. Si los discos están sin montar (Puppy), los monta de lectura y usa el que tenga `master.1000` |

### B. Canciones y catálogo
| # | Requerimiento | Prioridad | Estado |
|---|---|---|---|
| B1 | Importar todo el `Musica.data` correctamente (una fila por archivo, UTF-8) | P0 | ✅ 165,790 pistas en ~10 s; re-importar conserva números e historial |
| B2 | Agregar canciones copiando carpetas al disco, sin otro programa | P0 | ✅ escaneo al arrancar y desde el admin ([AGREGAR-CANCIONES.md](AGREGAR-CANCIONES.md)) |
| B3 | Escaneo de 1 TB sin congelar la UI, solo cambios | P0 | ✅ solo desde el admin (o si el catálogo está vacío), en segundo plano; la pantalla lee mientras el escaneo escribe en lotes de 2,000; va más despacio para no trabar el video; no oculta nada si el disco se desconecta a medias. Con 1 millón de archivos de prueba usa 40–180 MB de memoria (antes 550 MB), ver [PERFORMANCE.md](PERFORMANCE.md). 🔧 falta medir con el disco real |
| B4 | Archivos faltantes o dañados se ocultan sin trabar la rockola | P0 | ✅ faltante → oculto (no si el disco se desconectó); dañado → salta al siguiente |
| B5 | Detectar duplicados | P2 | ✅ `importcatalog -dupes` y en Diagnóstico (no borra nada) |
| B6 | Separar `Título - Artista` en carpetas de recopilación | P1 | 🟡 heurística: compara con el nombre de la carpeta y detecta recopilaciones (`#`, `$`, mix, éxitos…) |
| B7 | Reproducir todos los formatos del catálogo | P0 | ✅ mpv; CDG con su `.mp3`. 🟡 `.kar` (MIDI) no; las 41 líneas sin tipo/DAT se omiten |

### C. Búsqueda y navegación
| # | Requerimiento | Prioridad | Estado |
|---|---|---|---|
| C1 | Navegar con el teclado numérico (navegación propia) | P0 | ✅ tres columnas Letra · Artistas · Canciones; 8/2 dentro de la columna, 4/6 entre columnas, 9/3 saltos de 8 |
| C2 | Búsqueda por letra inicial | P0 | ✅ columna de letras siempre visible (ignora acentos y los `#`/`$` de las recopilaciones) |
| C3 | Número de canción fijo + catálogo impreso | P1 | ✅ `0 0 número Intro`; `importcatalog -export` genera el catálogo HTML para imprimir |
| C4 | Búsqueda de texto rápida sin acentos | P1 | ✅ FTS5, con teclado completo (escribir abre la búsqueda) |
| C5 | Filtro solo karaoke / música / videos | P1 | ✅ pantalla «¿Qué quieres hacer?» (Karaoke · Videos · Música) al abrir y con la tecla 1; los videos musicales ya no se marcan como karaoke |

### D. Reproducción y cola
| # | Requerimiento | Prioridad | Estado |
|---|---|---|---|
| D1 | Cola, saltar, pausa, volumen | P0 | ✅ |
| D2 | Volumen y ajustes sobreviven a un apagón | P0 | ✅ `data/state.json` con escritura atómica. Cambio: la cola ya no se guarda; al reiniciar empieza vacía y en silencio (el dueño prefirió eso a que vuelva la última canción de la sesión anterior) |
| D3 | Aviso "Siguiente: …" sobre el video | P1 | ✅ 10 s antes del final, no al empezar (al empezar solo "▶ canción") |
| D4 | Normalizar volumen entre canciones | P1 | ✅ `dynaudnorm` |
| D5 | Música de fondo cuando la cola está vacía | P1 | ✅ audio al azar de MUSICA después de la primera canción de la sesión; un pedido la interrumpe |
| D6 | Límite de canciones por persona/turno | P2 | 🟡 `max_queue` y `max_per_artist_in_row`; sin forma de identificar personas |

### E. Que se vea bien
| # | Requerimiento | Prioridad | Estado |
|---|---|---|---|
| E9 | Teclado numérico en pantalla | P0 | ✅ cada tecla dice qué hace en la columna activa y se ilumina al presionarla |
| E10 | Portadas rápidas | P1 | ✅ miniaturas de 240 px hechas una vez en `data/thumbs` (en segundo plano); la pantalla nunca lee el disco de música |
| E8 | Pantalla de bienvenida con avance de la carga | P1 | ✅ logo animado, negocio y mensaje; dice qué está haciendo hasta que el catálogo está listo |
| E7 | Reproductor integrado (no ventana aparte) | P0 | ✅ libmpv dibuja cada cuadro dentro de KaraokeNino: pantalla completa total, recuadro al elegir, diálogos y avisos encima; se sale con cualquier número (`make test-e2e`). Necesita `libmpv2` |
| E1 | Se adapta a cualquier TV (16:9, 4:3, 720p/1080p) | P0 | ✅ diseño escalado con barras; probado a 1024×768, 1280×720 y 1920×1080 |
| E2 | Diseño moderno, letras grandes, animaciones ligeras | P1 | ✅ skin `moderno` 1280×720 |
| E3 | Portadas en la lista y al reproducir audio | P1 | ✅ la de Musica.data, si no `images.jpg`/`cover.jpg`/…, si no otra imagen de la carpeta; el formato se reconoce por el contenido (muchas «.jpg» bajadas de internet son WebP/PNG/GIF); sin portada → iniciales |
| E4 | Pantalla de espera | P1 | ✅ a los 90 s: negocio, mensaje, lo más pedido, instrucciones, qué suena |
| E5 | Temas de color sin recompilar | P2 | ✅ moderno, boda, xv, infantil (config o admin) |
| E6 | Mensaje de bienvenida por evento | P2 | ✅ config o admin |

### F. Renta y administración
| # | Requerimiento | Prioridad | Estado |
|---|---|---|---|
| F1 | Menú admin con PIN; los invitados no pueden salir | P0 | ✅ Esc + PIN (configurable); cerrar ventana bloqueado |
| F2 | Volumen máximo por evento | P0 | ✅ |
| F3 | Quitar o reordenar canciones de la cola | P1 | ✅ |
| F4 | Modo infantil | P1 | ✅ |
| F5 | Historial y ranking por evento y total | P1 | ✅ tecla 7 y pantalla de espera; "Nuevo evento" en admin |
| F6 | Logo y contacto del negocio en pantalla | P2 | ✅ `business.*` |

### G. Mantenimiento
| # | Requerimiento | Prioridad | Estado |
|---|---|---|---|
| G1 | Todo configurable en `config.yaml` | P0 | ✅ [config.example.yaml](https://github.com/joemag50/KaraokeNino/blob/main/config/config.example.yaml) comentado |
| G2 | Log con rotación + diagnóstico en admin | P1 | ✅ 5 MB × 3; `kiosk.log` y `crash.log` también tienen tope |
| G3 | Actualizar el programa desde USB | P1 | ✅ `karaokenino-update/` con checksum y regreso automático a la versión anterior (probado en contenedor) |
| G4 | Pruebas automáticas | P1 | ✅ `make test`; `make test-linux` con mpv real |
| G5 | `make release` | P1 | ✅ genera `dist/*.tar.gz` + carpeta `karaokenino-update/`; instalado y abierto en Debian 12 (Xvfb, 1920×1080) |
| G6 | Repositorio git | P0 | ✅ local (sin remoto) |
| G7 | Tareas de mantenimiento | P1 | ✅ `importcatalog -mantenimiento`: revisión del catálogo, estadísticas, índice de búsqueda, historial → totales, faltantes, miniaturas huérfanas, registros, respaldos y espacio libre ([MANTENIMIENTO.md](MANTENIMIENTO.md)). 🟡 falta el botón en el admin |

---

## 3. Falta probar en la rockola real

Todo lo de arriba tiene pruebas automáticas (en macOS y en Linux con mpv real),
pero esto solo se puede comprobar con el hardware:

- Pantalla completa y teclado numérico reales en X11 (con y sin gestor de ventanas).
- Video: que el recuadro de vista previa y el cambio a pantalla completa de mpv se
  vean donde deben, y quién tiene el foco del teclado.
- Autologin + arranque (`install.sh --kiosk`) en la Debian/Puppy de la rockola.
- Salida de audio (HDMI vs. analógica) y rendimiento con 2 GB.
- Tiempo del primer escaneo del disco de 1 TB.

---

## 4. Preguntas abiertas
- ¿La rockola se queda sola en el evento o va alguien que la opera?
- ¿Cada renta lleva el mismo disco de 1 TB, o se arman discos distintos por cliente?
- ¿Se usa con TV del cliente o llevan su propia pantalla?
- ¿Hay que limitar el tiempo de renta en el software (p. ej. apagar a las 5 h), o eso se maneja aparte?
- ¿La computadora de la rockola es de 64 bits? (`make release ARCH=386` si es de 32).
