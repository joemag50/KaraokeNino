# Dependencias para compilar KaraokeNino (UI Fyne)

La interfaz nativa usa **OpenGL + X11** (o Wayland en algunas distros).

## Debian / Ubuntu (PC de desarrollo)

```bash
sudo apt install golang-go mpv \
  libgl1-mesa-dev xorg-dev libxcursor-dev libxrandr-dev \
  libxinerama-dev libxi-dev libxxf86vm-dev pkg-config
```

Compilar:

```bash
make build      # o: go build ./cmd/karaokenino ./cmd/importcatalog
make test
```

En macOS también compila y corre (útil para desarrollar con `demo_mode`).
Para el paquete de la rockola usa `make release` (Docker).

## Ejecutar

```bash
cp config/config.dev.yaml config/config.yaml   # sin videos en dev
./karaokenino -config config/config.yaml
```

Ventana de 1280×720 (o pantalla completa); la skin se escala a cualquier tamaño.

## Rockola offline

En la máquina del bar hace falta lo mismo para compilar **o** copiar el binario `karaokenino` ya compilado en una Debian igual (amd64).

`importcatalog` sigue pudiendo compilarse con `CGO_ENABLED=0` (solo SQLite puro).
