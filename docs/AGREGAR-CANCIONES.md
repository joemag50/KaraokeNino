# Agregar canciones

## Lo más fácil: copiar carpetas al disco

El disco de música tiene la estructura `master.1000/<género>/<artista>/<canción>`:

```text
master.1000/
  karaokes/
    Juan Gabriel/
      Amor Eterno.avi
      images.jpg            ← portada (opcional)
    Natalia Lafourcade/     ← carpeta nueva → artista nuevo en KARAOKES
      Hasta la Raíz.mp4
  cumbias/                  ← carpeta nueva → género nuevo "CUMBIAS"
    Los Ángeles Azules/
      Nunca Es Suficiente.mp3
```

1. Copia la carpeta del artista dentro del género (o crea un género nuevo).
2. En la rockola: Esc → PIN → **Re-escanear disco**. (La rockola no revisa el
   disco sola en cada encendido, para arrancar rápido; si el catálogo está vacío,
   sí lo revisa sola.)

El escaneo corre en segundo plano (la rockola se sigue usando) y:

- agrega solo los archivos **nuevos**; las canciones ya conocidas conservan su número;
- un artista nuevo junto a otros de `karaokes/` queda en **KARAOKES**; una carpeta
  nueva en la raíz se vuelve un **género nuevo**;
- **oculta** las canciones que ya no están en el disco y las vuelve a mostrar si regresan;
- si casi nada del catálogo está en el disco (¿disco equivocado?), o alguna
  carpeta no se pudo leer (¿se desconectó el disco?), no oculta nada;
- va más despacio para que el video que suena del mismo disco no se trabe, y
  escribe de 2,000 en 2,000: si se interrumpe, lo ya agregado se queda.

Formatos: MP3, WMA, WAV, M4A, OGG, FLAC, AVI, MPG, MPEG, MP4, MKV, WMV, FLV, VOB,
WEBM, MOV y CDG (con su `.mp3` del mismo nombre al lado).

Nombres: `Título.ext`, o `Título - Cantante.ext` en carpetas de recopilación
(las que empiezan con `#`, `$` o dicen "mix", "éxitos", "varios"…). Portada:
`images.jpg`, `cover.jpg`, `folder.jpg` o `portada.jpg` en la carpeta del artista.

## Desde la PC (sin ocupar la rockola)

```bash
./importcatalog -scan /ruta/al/disco/master.1000   # o -scan auto; -suave si la rockola toca de ese disco
./importcatalog -dupes                              # canciones repetidas
./importcatalog -export catalogo.html -genres KARAOKES -title "Karaokes"
```

`catalogo.html` es el **catálogo impreso** con el número de cada canción (los
invitados marcan `0 0 número Intro`). Ábrelo en el navegador e imprime.

Para llevar el catálogo listo a la rockola: copia `data/catalog.db` dentro de
`karaokenino-update/` en el disco (ver [AUTOSTART.md](AUTOSTART.md)).

## Desde una lista Musica.data (una vez)

```bash
./importcatalog -musica /ruta/a/Musica.data
```

Importa las 165,790 canciones con sus géneros en ~10 s. Se puede repetir: los
números y el historial se conservan, y lo agregado por escaneo no se toca.
