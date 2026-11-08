# Mantenimiento

La rockola se cuida casi sola: el registro rota (5 MB × 3), el catálogo se
optimiza al cerrar el programa y después de cada escaneo, y un escaneo nunca
oculta canciones si el disco se desconectó a medias. Lo de esta página es la
revisión de rutina, para que siga rápida y sin sorpresas aunque el disco tenga
terabytes y el historial años.

Todo se hace con `importcatalog`, en la carpeta de la rockola:

```bash
cd /opt/karaokenino
./importcatalog -mantenimiento
```

Se puede correr **con la rockola encendida** (sigue tocando y registrando lo
que se pide). Las únicas tareas que conviene hacer con la rockola sin uso son
`-vacuum` y `-purgar-faltantes` (ver abajo).

Al final dice qué hizo, una línea por tarea (`✓`), y lo que hay que revisar
(`!`). Si encontró algo que revisar, termina con código 1 (útil en scripts).

## Tareas de mantenimiento

### Antes de cada renta

- [ ] `./importcatalog -mantenimiento -disco auto` — catálogo sano, espacio
      libre en los dos discos. Si sale `!`, resuélvelo antes de salir.
- [ ] Si copiaste canciones: Esc → PIN → **Re-escanear disco** (o
      `./importcatalog -scan auto` con la rockola cerrada; es más rápido).
- [ ] Diagnóstico en el menú de administrador: disco encontrado, canciones,
      faltantes.

### Cada mes (o cada 10 eventos)

- [ ] `./importcatalog -respaldo auto -mantenimiento -revision-completa -disco auto`
      — respaldo del catálogo y revisión completa (tarda más: también revisa
      cada índice).
- [ ] `./importcatalog -faltantes` — canciones que no están en el disco hace
      más de 30 días. Si de verdad ya no existen (las borraste o cambiaste de
      disco), bórralas del catálogo (ver «Cada año»).

### Cada año (o cuando el diagnóstico diga que el catálogo creció mucho)

Con la rockola sin uso (Esc → PIN → **Salir al escritorio**):

```bash
./importcatalog -respaldo auto -mantenimiento -podar-historial 12 -purgar-faltantes -vacuum -disco auto
```

- Junta en el total las reproducciones de hace más de 12 meses: **«Lo más
  pedido» de siempre no cambia**; solo se pierde el ranking de eventos viejos.
- Borra del catálogo las canciones que faltan del disco hace más de 30 días.
- Compacta el archivo del catálogo.

### Si algo se ve mal

| Síntoma | Qué correr |
|---------|-----------|
| `! catálogo dañado` | Cierra la rockola. Copia el último respaldo (`data/respaldos/`) o `data/catalog.db.prev` sobre `data/catalog.db`, o vuelve a importar (`-musica`) y escanear (`-scan auto`). |
| La búsqueda no encuentra algo que sí está | `./importcatalog -mantenimiento -reconstruir-busqueda` |
| `! … casi lleno` (disco del sistema) | `./importcatalog -mantenimiento` ya borra miniaturas y registros viejos; si sigue lleno, borra respaldos viejos o descargas de la PC. |
| `! … casi lleno` (disco de música) | Ya no caben canciones nuevas: borra duplicadas (`-dupes`) o usa un disco más grande. |
| La rockola se siente lenta tras un escaneo grande | `./importcatalog -mantenimiento` (actualiza las estadísticas y compacta la búsqueda). |

Para ver qué haría sin cambiar nada: agrega `-simular`.

## Qué hace cada tarea

| Opción | Qué hace | Tarda (166 mil canciones) |
|--------|----------|------|
| `-mantenimiento` | Revisión rápida del catálogo (`quick_check`); compacta el índice de búsqueda; actualiza las estadísticas de consultas (`ANALYZE`); reporta canciones faltantes; borra miniaturas de artistas que ya no están y registros rotados de más de 90 días; recorta registros que solo crecen (`kiosk.log`, `crash.log`) a menos de 8 MB; deja los 5 respaldos más nuevos; dice el espacio libre. **No borra canciones ni historial.** | 1–3 s en una PC; algo más en la rockola |
| `-revision-completa` | En vez de la revisión rápida, la completa (`integrity_check`, cada índice y el de búsqueda). | 2–10 s |
| `-disco RUTA` / `-disco auto` | También dice el espacio libre del disco de música. | — |
| `-podar-historial N` | Pasa las reproducciones de hace más de N meses al total por canción (`play_totals`) y las borra. | < 1 s |
| `-faltantes` | Lista las canciones ocultas porque su archivo no está en el disco hace más de `-dias-faltantes` (30). | — |
| `-purgar-faltantes` | Borra esas canciones del catálogo (y su historial). Los números de las demás canciones **no cambian** y no se reutilizan. Hazlo solo con el disco correcto conectado. | < 1 s |
| `-vacuum` | Reescribe el catálogo sin espacio libre. Necesita espacio libre para una copia del catálogo; mientras corre no se registran reproducciones. | 2–10 s |
| `-reconstruir-busqueda` | Rehace el índice de búsqueda desde las canciones. | 1–5 s |
| `-respaldo auto` / `-respaldo ARCHIVO` | Copia consistente del catálogo (aunque la rockola esté tocando) en `data/respaldos/catalog-FECHA.db`. | 1–3 s |
| `-simular` | Dice qué haría, sin cambiar nada. | — |

Lo que la rockola hace sola, sin que lo pidas:

- `PRAGMA optimize` al cerrar el programa y después de un escaneo con cambios.
- El registro `data/logs/karaokenino.log` rota a 5 MB y guarda 3 copias.
- `crash.log` se recorta a 512 KB al arrancar si pasa de 1 MB; `kiosk.log`
  rota a 5 MB (una copia, `kiosk.log.1`).
- El escaneo escribe de 2,000 en 2,000 canciones: si se interrumpe (apagón,
  disco desconectado), lo ya escrito se queda y el siguiente escaneo sigue
  donde faltaba. Si alguna carpeta no se pudo leer, **no oculta nada**.

## Para el menú de administrador (programadores)

Todo está en funciones que la pantalla puede llamar en segundo plano:

| Función | Uso |
|---------|-----|
| `maint.Run(ctx, store, maint.Options{…})` | La rutina completa; devuelve `Report{Lines, Problems}` en español, listo para mostrar. Con `Options{ThumbsDir, LogDir, MusicRoot}` es la rutina segura. |
| `maint.CleanThumbs(ctx, store, dir, dryRun)` | Borra miniaturas sin artista (se niega si casi ninguna coincide). |
| `maint.DiskFree(path)` | Espacio libre y total del disco de esa ruta. |
| `maint.PruneBackups(dir, keep, dryRun)` | Deja los `keep` respaldos más nuevos. |
| `store.QuickCheck(ctx)` / `store.IntegrityCheck(ctx)` | `nil` = sano. |
| `store.Optimize(ctx)` / `store.Analyze(ctx)` | Estadísticas del planificador (rápido / completo). |
| `store.OptimizeSearch(ctx)` / `store.RebuildSearchIndex(ctx)` | Índice de búsqueda. |
| `store.PrunePlays(ctx, antesDe)` | Historial → totales. |
| `store.MissingTracks(ctx, antesDe, límite)` / `store.PurgeMissing(ctx, antesDe)` | Faltantes. |
| `store.Vacuum(ctx)` / `store.Backup(ctx, archivo)` / `store.FileSizes(ctx)` | Archivo del catálogo. |
| `logx.Tidy(dir, edadMáx, bytesMáx, dryRun)` / `logx.CapFile(ruta, bytesMáx)` | Registros. |
| `catalog.ScanWith(ctx, store, raíz, catalog.ScanOptions{Busy: …})` | Escaneo que va más despacio mientras `Busy()` diga que algo está sonando. |
