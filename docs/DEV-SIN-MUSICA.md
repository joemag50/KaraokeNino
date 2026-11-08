# Desarrollo en una PC sin archivos de música/video

En la computadora de desarrollo basta con el **catálogo** (`Musica.data` importado);
no hace falta el disco de 1 TB.

```bash
make catalog MUSICA=/ruta/Musica.data         # importa Musica.data → data/catalog.db
cp config/config.dev.yaml config/config.yaml  # demo_mode: true, ventana normal
./karaokenino
```

| Función | Sin disco |
|---------|-----------|
| Navegar letras, artistas, canciones, número, búsqueda | Sí |
| Cola, "Lo más pedido", admin, pantalla de espera | Sí |
| Reproducir | Simulado: cada canción "suena" 10 s (`demo_mode`) |
| Portadas | No (salen las iniciales del artista) |

## Ver las pantallas sin abrir la ventana

```bash
make snapshots    # PNG de cada pantalla en snapshots/ (1280x720, 1920x1080, 1024x768)
```

## Probar con mpv de verdad

`make test-linux` corre todas las pruebas en un contenedor Debian con mpv real:
reenvío de teclas desde el video, progreso, volumen máximo y rutas con mayúsculas
distintas (Linux distingue `.AVI` de `.avi`).

## En la rockola

Mismo programa con `demo_mode: false`. El disco se encuentra solo
(`library.roots`, o una carpeta `master.1000` bajo `/media`, `/mnt`, `/run/media`).
