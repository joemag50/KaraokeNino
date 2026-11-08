# Cómo deben estar las carpetas

KaraokeNino usa tres lugares: el **disco de música** (las canciones), la
**carpeta del programa** en la rockola (programa, configuración y datos) y,
cuando se actualiza, una carpeta **`karaokenino-update/`** en el disco o una USB.

El catálogo (`catalog.db`), la cola y los registros **no están en este repo**:
se crean en la rockola (`data/`), y el catálogo se arma escaneando el disco o
importando una lista `Musica.data` ([AGREGAR-CANCIONES.md](AGREGAR-CANCIONES.md)).

## 1. Disco de música

Una carpeta `master.1000` (el nombre se cambia en `library.dir_name`) en
cualquier disco: se busca sola en `/media`, `/mnt` y `/run/media`, y si el disco
no está montado (Puppy) se monta de solo lectura.

```text
master.1000/
├── karaokes/                    ← género (carpeta de primer nivel)
│   ├── Juan Gabriel/            ← artista
│   │   ├── Amor Eterno.avi      ← canción: el nombre del archivo es el título
│   │   ├── Querida.mpg
│   │   └── images.jpg           ← portada del artista (opcional)
│   ├── Café Tacvba/
│   │   ├── Eres.mp4
│   │   ├── Las Flores.cdg       ← karaoke CDG: lleva su .mp3 con el mismo nombre
│   │   ├── Las Flores.mp3
│   │   └── cover.jpg
│   └── # Éxitos 2026/           ← recopilación (empieza con #, $ o dice mix/éxitos/varios)
│       └── Hombre libre - La Adictiva.avi   ← «Título - Cantante»
├── videos/
│   └── Shakira/
│       └── Antología.mp4
├── musica/
│   └── Celia Cruz/
│       ├── La Vida Es Un Carnaval.mp3
│       └── folder.jpg
├── infantiles/
│   └── Cri-Cri/
│       └── La Patita.mp3
└── karaokenino-update/          ← solo para actualizar (ver 3); se puede borrar después
```

- **Género** = carpeta de primer nivel (en el menú se ve en mayúsculas: KARAOKES,
  MUSICA…). **Artista** = carpeta dentro del género. **Canción** = archivo.
- Todo lo que está en un género cuyo nombre dice **«karaoke»** es **karaoke**.
  En los demás géneros, un archivo con video (AVI, MPG, MP4, MKV, WMV…) es
  **video** y uno de audio (MP3, WMA, FLAC…) es **música**.
- **Portada**: `images.jpg`, `cover.jpg`, `folder.jpg`, `picture.jpg` o
  `portada.jpg` (también `.png`/`.bmp`) en la carpeta del artista. Puede ser
  JPEG, PNG, GIF, BMP o WebP aunque se llame `.jpg`. AVIF y HEIC no se leen.
- Mayúsculas y minúsculas no importan (`IMAGES.JPG` = `images.jpg`): los nombres
  vienen de Windows.

## 2. Carpeta del programa (en la rockola)

`install.sh` la deja en `/opt/karaokenino` (se cambia con `KARAOKE_ROOT`):

```text
/opt/karaokenino/
├── karaokenino                  ← el programa
├── importcatalog                ← importar, escanear, mantenimiento, catálogo impreso
├── config/
│   └── config.yaml              ← configuración del evento y la máquina (de config.example.yaml)
├── deploy/                      ← kiosk-start.sh, update.sh, install.sh
├── docs/
└── data/                        ← lo crea el programa; NO va en el repo
    ├── catalog.db               ← catálogo: canciones, números, «lo más pedido»
    ├── catalog.db.prev          ← el anterior, al actualizar con un catálogo nuevo
    ├── state.json               ← volumen y ajustes del admin (la cola empieza vacía)
    ├── thumbs/                  ← miniaturas de portadas (240 px), se hacen una vez
    │   └── grandes/             ← portadas grandes (720 px) para la pantalla de música
    └── logs/
        ├── karaokenino.log      ← lo que pasa (con rotación)
        ├── kiosk.log            ← arranques del kiosco
        └── crash.log
```

Las rutas de `data/` se cambian en `config.yaml` (`catalog_db`, `state_file`,
`log_file`); las miniaturas van junto al catálogo.

## 3. Actualizar sin internet

`make release` arma `dist/karaokenino-<versión>-linux-amd64/`. Para actualizar,
copia su carpeta `karaokenino-update/` a la raíz del disco de música (o de una
USB) y reinicia la rockola:

```text
karaokenino-update/
├── karaokenino                  ← programa nuevo
├── karaokenino.sha256           ← se verifica antes de instalar
├── kiosk-start.sh
├── update.sh
└── catalog.db                   ← opcional: un catálogo armado en la PC
```

Se instala una sola vez, se guarda la versión anterior y, si la nueva falla tres
veces seguidas al arrancar, se regresa sola a la anterior ([AUTOSTART.md](AUTOSTART.md)).
