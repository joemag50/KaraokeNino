# Instalar en la rockola y arranque automático

## 1. Preparar el paquete (en una PC con Docker)

```bash
make release            # dist/karaokenino-<versión>-linux-amd64.tar.gz
make release ARCH=386   # para computadoras de 32 bits
```

Se compila sobre Debian 12 bookworm (glibc 2.36): corre en Debian 12+, en Puppy
basados en bookworm y en Ubuntu 22.04+. (Debian 11 ya no tiene soporte.)

## 2. Instalar (en la rockola, sin internet)

Requisitos: `mpv`, `libmpv2` (video integrado) y X11 (ver [AUDIO-VIDEO-DRIVERS.md](AUDIO-VIDEO-DRIVERS.md)).

```bash
tar xzf karaokenino-*-linux-amd64.tar.gz
cd karaokenino-*-linux-amd64
sudo ./deploy/install.sh                 # opción A: arranca al iniciar el escritorio
sudo ./deploy/install.sh --kiosk rockola # opción B: sin escritorio (recomendada para rentar)
```

Copia el catálogo ya importado si lo tienes (`data/catalog.db` → `/opt/karaokenino/data/`),
o deja que la rockola **escanee el disco** sola al arrancar.

| Modo | Qué hace |
|------|----------|
| **Escritorio** | `~/.config/autostart/karaokenino.desktop`. Activa el inicio de sesión automático del escritorio (LightDM: `autologin-user=…`). |
| **Kiosco** (`--kiosk USUARIO`) | Autologin en tty1 → `startx` solo con KaraokeNino (sin escritorio ni menú al que escapar), cursor oculto, y permiso para apagar/reiniciar desde el admin. |
| **Puppy** (detectado solo) | Enlace en `/root/Startup/karaokenino`. |

Después edita `/opt/karaokenino/config/config.yaml`: **cambia `admin.pin`**, el nombre de tu negocio y el contacto.

## 3. Qué pasa al encender

`kiosk-start.sh` es un ciclo que:

1. Instala una actualización de la USB si hay (`update.sh`, ver abajo).
2. Arranca `karaokenino` en pantalla completa. La pantalla no se apaga (`xset -dpms`).
3. KaraokeNino busca el disco de música hasta 60 s (las USB tardan en montarse),
   recupera la cola que había si se fue la luz y revisa el disco en segundo plano.
4. Si el programa se cierra o falla, **lo vuelve a abrir** a los 2 s.
5. Si una versión nueva falla 3 veces seguidas al arrancar, **regresa a la anterior**.

Registro: `/opt/karaokenino/data/logs/` (`kiosk.log` y `karaokenino.log`, con rotación).

## 4. Actualizar sin internet

`make release` deja lista la carpeta `dist/<paquete>/karaokenino-update/`. Cópiala a la
**raíz del disco de música** (o cualquier USB) y reinicia la rockola (o "Reiniciar programa"
en el admin). Se verifica el checksum, se instala una sola vez y se guarda la versión
anterior. Opcional dentro de esa carpeta: `assets/` (skins) y `catalog.db`.

## 5. Salir y apagar

Solo desde el menú de administrador (Esc + PIN): *Reiniciar programa*, *Reiniciar
computadora*, *Apagar rockola* o *Salir al escritorio* (detiene el ciclo del kiosco).
Cerrar la ventana con el ratón no hace nada.
