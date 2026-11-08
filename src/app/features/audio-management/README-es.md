# Función de gestión de audio en Angular

Esta función independiente (standalone) proporciona una biblioteca de audio orientada al administrador sin depender del enrutador, el entorno, el servicio de traducción ni la implementación de autenticación de Amerimon. La aplicación anfitriona proporciona su URL de API, traducciones y autorización a nivel de ruta. La configuración de las pistas y el estado de activación se almacenan en el backend, no en la sesión del navegador: la programación de la reproducción continúa después de que el administrador cierra la sesión o la página, y permanece activa hasta que un administrador deshabilita o elimina la pista. Una transmisión única (broadcast) sigue siendo una sola reproducción por diseño.

## Instalación

Importa `AudioManagementComponent` y llama a `provideAudioManagement` desde la ruta de la aplicación anfitriona:

```ts
{
  path: 'admin/audio',
  canActivate: [hostAdminGuard],
  providers: [
    provideAudioManagement('/api/audio'),
    {
      provide: AUDIO_MANAGEMENT_TRANSLATE,
      useFactory: () => {
        const i18n = inject(HostTranslationService);
        return (key: string) => i18n.translate(key);
      },
    },
  ],
  loadComponent: () => import('./audio-management').then(m => m.AudioManagementComponent),
}
```

`provideAudioManagement` acepta la URL base para los endpoints de audio (que terminan en `/audio` en esta API). Su traductor por defecto devuelve la clave; las aplicaciones deben proporcionar `AUDIO_MANAGEMENT_TRANSLATE` y las traducciones para las claves `AUDIO_*` en la plantilla del componente.

`audioLabel` es impura, por lo que las etiquetas y descripciones emergentes (tooltips) existentes se traducen nuevamente cuando cambia el idioma del anfitrión o terminan de cargarse los diccionarios. Para los anfitriones basados en signals, el traductor proporcionado debe leer la señal de revisión de idioma/diccionario para notificar a la vista; el adaptador de Amerimon lee `TranslationService.revision()`. Esto mantiene la función independiente del servicio de traducción del anfitrión.

Opcionalmente, proporciona `AUDIO_MANAGEMENT_MONITOR_URL` con una URL de mismo origen (same-origin) relativa a la URL base de la aplicación para mostrar el botón de la ventana de monitor independiente. Por defecto, el acceso directo está oculto, lo que mantiene la función independiente de las rutas de Amerimon. Amerimon proporciona `allstarlink` en su ruta de audio protegida para administradores; el acceso directo de AllStarLink al audio se muestra únicamente a los administradores. Las ventanas emergentes (popups) no navegan en la página original y reportan explícitamente el bloqueo del navegador. No transfieren tokens: una sesión no recordada puede requerir un inicio de sesión independiente en la nueva ventana.

## Contrato de API

La función espera:

- `GET {baseUrl}/tracks` que devuelva `{ status, data: AudioTrack[] }`
- `POST {baseUrl}/tracks` con campos multiparte (multipart) `name`, `playback_mode`, `interval_minutes` (para el modo de silencio) y `file`
- `PATCH {baseUrl}/tracks/{id}` con campos de pista editables
- `DELETE {baseUrl}/tracks/{id}`
- `POST {baseUrl}/tracks/{id}/play`
- La configuración de segmentos se persiste por pista mediante `PATCH /tracks/:id` con `{ start_ms, end_ms }` (milisegundos enteros, inicio inclusivo/fin exclusivo, `0 <= start_ms < end_ms <= duration_ms`). `{ start_ms: 0, end_ms: 0 }` restablece el audio completo. Ambos límites son obligatorios a la vez. Las pistas existentes toman el audio completo por defecto mediante la migración `0015`.
- El diálogo de completo/segmento utiliza una línea de tiempo (timeline) con dos controladores accesibles por teclado y una selección resaltada, además de campos numéricos de inicio/fin en segundos (precisión de 0.001 s). Los temas globales de entrada de texto deben excluir `input[type=range]` para que los controles deslizantes transparentes superpuestos no oculten el riel ni el controlador del otro. Guarda la configuración sin transmitir, o transmite para guardar y luego emitir. Los mismos límites guardados se aplican a la reproducción manual y programada, incluso después de recargas/reinicio; cambiar los límites no restablece el temporizador de intervalos. Cancelar descarta las ediciones no guardadas. El restablecimiento debe guardarse.
- `POST /tracks/:id/play` con un cuerpo vacío utiliza los límites guardados; los límites explícitos los anulan solo para esa solicitud (las solicitudes `0/0` completan el audio). Ambas rutas de reproducción comparten el corte de PCM temporal y las reservas basadas en duración. El archivo original, la vista previa local y la descarga permanecen completos. Las etiquetas y la validación utilizan claves de traducción ES/EN.
- El diálogo de selección tiene su propia vista previa local utilizando la descarga autenticada de la pista y `audioSegmentBlob` para cortar muestras PCM normalizadas exactas en un archivo WAV exclusivo del navegador. Reutiliza el reproductor con medidor de nivel, nunca llama a los endpoints de guardado/reproducción, oculta/detiene la reproducción obsoleta cuando cambian los límites y revoca las URLs de objeto al reemplazar, cerrar o destruir. Las vistas previas/descargas ordinarias de la biblioteca siguen conteniendo la pista completa.
- Las tarjetas de la biblioteca resaltan los límites guardados con una insignia traducida de **Segmento seleccionado** que muestra el inicio, el fin y la duración. Las ediciones de diálogo no guardadas no modifican la tarjeta. Guardar un restablecimiento elimina la insignia y restaura la indicación de **Audio completo**.
- `GET {baseUrl}/tracks/{id}/file` que devuelve un blob WAV autenticado para la escucha local
- `GET {baseUrl}/recordings` que devuelve `{ status, data: AudioRecording[] }`
- `POST {baseUrl}/recordings` con `{ name }`, confirmado después de que comience a llegar el PCM del lado del servidor
- `PATCH {baseUrl}/recordings/{id}` con `{ action: 'pause' | 'resume' | 'stop' }`
- `GET {baseUrl}/recordings/{id}/file` que devuelve un blob WAV autenticado
- `DELETE {baseUrl}/recordings/{id}`
- `POST {baseUrl}/recordings/{id}/library` creando una copia de pista independiente sin transmitirla

El anfitrión sigue siendo responsable de la autenticación/autorización, los interceptores de API y la navegación. Las cargas aceptan WAV, MP3, FLAC, OGG/Opus, M4A/MP4, AAC, WMA, AIFF, WebM, MKA, AMR y APE. El backend realiza la conversión a PCM WAV mono de 16 bits y 8 kHz a través de FFmpeg instalado, con límites de entrada/salida de 25 MiB, una conversión a la vez y un tiempo de espera (timeout) de 60 segundos. La salida que alcanza el límite se rechaza, no se trunca silenciosamente. Las cargas WAV compatibles conservan la ruta de normalización en proceso existente. Los archivos inválidos, la falta de FFmpeg, los códecs no compatibles, una conversión ocupada y el tiempo de espera devuelven códigos de error traducidos.

El componente utiliza claves de traducción para cada etiqueta, confirmación, aviso de éxito y error. El anfitrión puede proporcionar `AUDIO_MANAGEMENT_TRANSLATE` directamente o pasar un traductor a `provideAudioManagement`. Los errores del backend incluyen un `code` estable (`AUDIO_ERROR_*`) además de un `message` de diagnóstico; el componente muestra el código traducido y nunca asume que el texto del backend coincide con el idioma de la aplicación anfitriona.

La biblioteca muestra intencionalmente las pistas deshabilitadas para que se puedan activar más tarde. Los modos de reproducción compatibles son `broadcast_once` y `broadcast_silence`; cambia el modo y el intervalo de una pista en su tarjeta de biblioteca y luego guarda los cambios explícitamente. Una pista con `broadcast_silence` pendiente permanece en espera mientras cualquier nodo monitoreado esté activo y se reproduce una vez que todos los nodos monitoreados estén en silencio. La reproducción programada se serializa utilizando una reserva persistente basada en la duración WAV verificada. Eliminar una pista es explícito y elimina su archivo y programación. Las pistas existentes con `on_connection` se migran a `broadcast_once` y siguen estando disponibles para su reproducción activada por el administrador.

Las solicitudes en primer plano comparten un estado de ocupado (busy) de nivel de servicio en toda la biblioteca y las grabaciones. Mientras una acción está pendiente, los demás botones de acción y campos editables se deshabilitan. El texto de progreso (incluido el porcentaje de carga) aparece solo dentro del botón iniciador con `aria-busy`; los errores y avisos de finalización permanecen separados. El sondeo (polling) de grabación en segundo plano no bloquea la pantalla, y una grabación en curso no impide utilizar sus controles de pausa/detención.

Los botones de acción utilizan iconos SVG en línea con descripciones emergentes traducidas y etiquetas accesibles ocultas visualmente. Las acciones pendientes muestran un indicador de carga (spinner) y su texto de proceso dentro del botón. El botón de auriculares previsualiza cualquier pista de biblioteca localmente, incluidas las pistas deshabilitadas o programadas, utilizando el reproductor reutilizable y el medidor de señal. Solo descarga el WAV autenticado y nunca envía un comando AMI ni cambia una programación. El icono de transmisión distintivo conserva la reproducción manual de Asterisk para las pistas únicas habilitadas. Las URLs de vista previa se revocan al cambiar de pista, eliminar la pista previsualizada o abandonar la pantalla.

El icono de descarga guarda el archivo WAV almacenado de las pistas de la biblioteca o de las grabaciones finalizadas que no estén vacías a través de los endpoints de blob autenticados. Nunca transmite audio ni cambia la activación/programación. La conversión no conserva la carga comprimida original; las descargas utilizan el formato WAV compatible con Asterisk. El progreso de descarga permanece dentro de su botón iniciador y las URLs de descarga temporal se revocan después del envío.

## Grabaciones de nodos del lado del servidor

`AudioRecordingsComponent` se incluye en la pantalla principal y también se puede importar por separado con los mismos proveedores. Captura la mezcla del nodo principal, no el micrófono del navegador. Un plan de marcado (dialplan) dedicado `amerimon-record-node` utiliza `Rpt(node,Pm)` en modo de monitorización de teléfono únicamente para incluir estaciones enlazadas; espiar únicamente el `rxchannel` de radio puede producir silencio mientras los enlaces están activos. El contexto del canal de radio Listen Live existente no cambia. El backend reutiliza el transporte EAGI, graba sin un WebSocket del navegador y persiste las grabaciones en un directorio privado junto a SQLite. Se admiten el inicio, la pausa manual, la reanudación, la detención, la reproducción exclusiva del navegador, la eliminación confirmada y la copia en la biblioteca de transmisión. La copia no transmite audio y la eliminación de una grabación no borra su copia en la biblioteca. Actualiza el backend instalado y el plan de marcado a través del instalador; las capturas que ya se estén ejecutando deben detenerse y reiniciarse para utilizar la nueva fuente.

La eliminación automática de silencios utiliza aproximadamente −40 dBFS PCM RMS, un pre-roll de 200 ms y un margen de liberación de 500 ms. La pausa manual tiene prioridad sobre la reanudación automática. El servidor limita cada grabación a 25 MiB y finaliza cuando se alcanza el límite. Cerrar el componente solo detiene el sondeo y revoca las URLs de blob del navegador; no detiene la captura. Si EAGI se desconecta, la grabación se interrumpe; tras reiniciar el servicio, el PCM persistido se recupera en un archivo WAV reproducible sin reanudar automáticamente la captura.

Todas las solicitudes de grabación deben utilizar el cliente/interceptores HTTP autenticados del anfitrión. La reproducción descarga un blob en lugar de incrustar una URL de servidor no autenticada. El elemento hijo notifica `AudioManagementService.libraryChanged` después de una copia, actualizando la biblioteca; el sondeo se desecha al destruir el componente. Proporciona traducciones para `AUDIO_RECORD_*` así como para las claves de audio existentes.

### Medidores de señal

Las grabaciones activas exponen `level_dbfs` en la API de listado: PCM RMS en dBFS, con un límite inferior de -60, una retención máxima de 500 ms y un tiempo de espera de inactividad de un segundo. Mide la señal entrante incluso durante la pausa manual, no el archivo WAV retenido. La pantalla realiza sondeos cada 500 ms durante la grabación (cada dos segundos cuando está inactiva); los niveles son transitorios y no requieren migración de base de datos.

La reproducción en el navegador de las grabaciones guardadas utiliza `AudioPlayerComponent` y el muestreo RMS de Web Audio cada 50 ms. No representa la salida de transmisión de Asterisk ni el volumen físico del altavoz. La pausa/finalización restablece el medidor, y destruir el reproductor cierra su contexto de audio. `AudioLevelMeterComponent` también se puede reutilizar por separado con las entradas `dbfs` y `label`. Ambos componentes se exportan y requieren traducciones `AUDIO_LEVEL_*`. No se necesita ningún permiso de micrófono ni dependencia adicional.
