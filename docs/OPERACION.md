# Operación en renta

## Antes de llevar la rockola

- [ ] `./importcatalog -mantenimiento -disco auto` sin avisos `!` ([MANTENIMIENTO.md](MANTENIMIENTO.md)).
- [ ] `admin.pin` cambiado en `config.yaml` (el diagnóstico avisa si sigue el de fábrica).
- [ ] Nombre del negocio y contacto (`business.name`, `business.contact`, `business.logo`).
- [ ] Probar con la TV del cliente: la pantalla se adapta sola a 4:3 o 16:9.

## En el evento (menú de administrador)

Presiona **Esc** y escribe el PIN + **Intro** (si hay una ventana abierta, la
primera Esc la cierra). Si el teclado de la rockola no tiene Esc, cambia la tecla en
`admin.open_sequence` (p. ej. `["KEY_KPSLASH", "KEY_KPSLASH", "KEY_KPSLASH"]` = / / / rápido).
Tres intentos fallidos cierran el diálogo y hay que esperar 1 minuto (luego 2, 4…
si siguen fallando).

| Opción | Uso |
|--------|-----|
| Volumen máximo | Tope para invitados (4/6). Se recuerda. |
| Volumen actual | |
| Administrar cola | 4/6 mover, Intro quitar |
| Saltar / Vaciar la cola | Vaciar pide confirmar (Intro dos veces) |
| Modo infantil | Solo muestra `z INFANTILES` (configurable en `admin.kids_genres`) |
| Música de fondo | Canciones al azar de MUSICA cuando nadie pide (empieza después de la primera canción del evento, nunca al encender); cualquier pedido la interrumpe |
| Tema de colores | moderno (neón de antro), boda (champaña y oro), xv (fucsia, violeta y destellos dorados), infantil (colores de dulce); al reiniciar el programa. Las animaciones se apagan con `ui.animations: false` en la configuración |
| Mensaje de bienvenida | "¡Felices 15, Ana!" en pantalla (necesita teclado completo; también en `event.welcome`) |
| Nuevo evento | Reinicia "Lo más pedido" para esta fiesta |
| Re-escanear disco | Después de copiar canciones nuevas (la rockola no lo hace sola al encender) |
| Diagnóstico | Disco, espacio libre, canciones, faltantes, duplicadas, memoria, registro |
| Reiniciar / Apagar / Salir al escritorio | Piden confirmar |

## Qué ve el invitado

- Pantalla principal: Letra · Artistas · Canciones, el teclado dibujado con lo que hace cada tecla, y cada canción con su número y si es KARAOKE/VIDEO/AUDIO.
- Bajo las canciones, **«Siguen en la cola»**: las próximas canciones (número, título, cantante) y «+N más»; **/** muestra la cola completa.
- Una barra delgada abajo de la pantalla muestra cuánto lleva la canción.
- Con un video, **7** cambia entre **Reproducción** (pantalla completa, para cantar) y **Lista**
  (el menú con el video en el recuadro, para pedir otra). En la lista, la tecla 7 dice «Cantar»
  (o «Ver video») y el video regresa solo a pantalla completa tras **10 s** sin teclas.
  En pantalla completa un aviso «7 ver la lista…» aparece unos segundos y se quita para no tapar la letra;
  queda solo una línea tenue de avance en el borde de abajo.
- Sobre el video: "▶ canción" al empezar y **"Siguiente: …"** 10 s antes del final (no al empezar).
- Sin tocar nada 90 s: **pantalla de espera** con tu negocio, el mensaje del evento,
  "Lo más pedido", cómo pedir canciones y qué está sonando.
- Límites opcionales: `max_queue` (cola máxima) y `max_per_artist_in_row`.
- A prueba de invitados necios: la misma canción no entra dos veces a la cola;
  saltar pide **.** dos veces y no se puede saltar otra en 5 s; una pausa o el
  volumen en 0 quedan indicados abajo hasta corregirse.

## Si algo falla

| Síntoma | Qué pasa / qué hacer |
|---------|----------------------|
| Se fue la luz / se reinició | Enciende **en silencio** con la **cola vacía** y el volumen y los ajustes igual. La cola de antes no se guarda (así no vuelve a sonar la última canción de la fiesta anterior): pidan sus canciones otra vez. |
| No salen portadas | En `data/logs/karaokenino.log` busca `miniaturas:`: dice cuántas no se pudieron hacer y por qué (formato no soportado, imagen dañada, demasiado grande, no se pudo leer), con un ejemplo de cada una. |
| "No se encontró el disco" | Conecta el disco; se detecta solo cada 10 s. Los discos sin montar (Puppy) se montan solos de lectura en `/mnt/sdXN` y se usa el que tenga `master.1000` (`library.auto_mount`). |
| Un archivo dañado | Se salta a la siguiente; si falta el archivo, se oculta del catálogo. |
| El programa se cerró | El kiosco lo vuelve a abrir en 2 s. |
| No hay sonido | Admin → Diagnóstico; revisa `mpv_args` (`--audio-device=…`) y [AUDIO-VIDEO-DRIVERS.md](AUDIO-VIDEO-DRIVERS.md). |

Ranking total desde la PC: el historial queda en `data/catalog.db` (tabla `plays`).
