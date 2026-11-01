# Listen Live

Listen Live permite escuchar en el navegador el audio de una extensión SIP o del nodo local, en tiempo real. Asterisk captura el audio; el backend Go lo transporta por WebSocket; el frontend lo procesa y reproduce.

## Vista general

```mermaid
flowchart LR
    User[Usuario] -->|Inicia escucha| Frontend[Frontend Angular]
    Frontend -->|POST /api/listen-live/start<br/>ext + UUID| API[API Go / Echo]
    API -->|Originate por AMI| Asterisk[Asterisk]
    Asterisk -->|ChanSpy| Target[Canal SIP o nodo]
    Asterisk -->|EAGI + audio_eagi.sh| TCP[Audio TCP :8086]
    TCP -->|PCM SLIN16| Stream[AudioStreamManager]
    Frontend <-->|WebSocket binario<br/>/api/listen-live/ws?uuid=...| Stream
    Frontend -->|Audio procesado| Speaker[Audio del navegador]
    Frontend -->|PATCH fecha_fin al cerrar| DB[(SqLite3: listenlive)]
```

El UUID correlaciona el flujo de audio de Asterisk con el WebSocket. La sesión de escucha de SqLite3 se identifica por `listenliveid` y pertenece a un usuario; son identificadores con funciones distintas.

## Inicio y recorrido del audio

```mermaid
sequenceDiagram
    autonumber
    actor U as Usuario
    participant F as Frontend
    participant API as Backend Go
    participant DB as SqLite3
    participant AMI as Asterisk AMI
    participant AST as Asterisk / EAGI
    participant TCP as Listener TCP :8086

    U->>F: Pulsa "Escuchar"
    opt Registrar sesión de usuario
        F->>API: POST /api/listen-live/sessions
        API->>DB: INSERT listenlive
        DB-->>API: listenliveid
        API-->>F: Sesión creada
    end
    F->>F: Genera UUID
    F->>API: POST /api/listen-live/start {ext, uuid}
    API->>AMI: Originate Local/{ext}@listen-live-spy/n
    AMI->>AST: Ejecuta listen-live-spy y listen-live-eagi
    API-->>F: Inicio aceptado
    F->>API: GET /api/listen-live/ws?uuid={uuid}
    API-->>F: Upgrade a WebSocket
    AST->>AST: ChanSpy captura el canal objetivo
    AST->>TCP: EAGI ejecuta audio_eagi.sh
    TCP->>TCP: Lee UUID y tramas PCM
    loop Mientras llega audio
        TCP->>API: Audio PCM SLIN16 (320 bytes / 20 ms)
        API-->>F: Mensaje WebSocket binario
        F->>F: Decodifica y reproduce el audio
    end
```

La configuración activa en el backend es `ListenModeEAGI`. En este modo, el script `audio_eagi.sh` envía primero el UUID (36 bytes) y luego el audio al listener TCP. Cada trama habitual tiene 320 bytes: PCM SLIN16, 8 kHz, aproximadamente 20 ms.

## Requisitos PJSIP y dialplan

Para escuchar una extensión, esta debe existir como endpoint PJSIP en `pjsip.conf` y el módulo `res_pjsip` debe estar disponible en Asterisk. Amerimon puede administrar endpoints desde **Admin → Extensiones PJSIP**; las extensiones nuevas usan por defecto el contexto `sip-phones`. La configuración de módulo y el contexto de teléfonos se instalan de forma aditiva mediante `pjsip-modules-patch.conf` y `pjsip-extensions-sip-phones-patch.conf`. Consulta la sección **PJSIP para teléfonos SIP** del [manual técnico](manual-tecnico) para el flujo de instalación, respaldos y resolución de conflictos con rutas existentes.

El parche PJSIP enruta las llamadas del teléfono por un canal Local hasta `rpt-internal` antes de entrar a `app_rpt`. No reemplaza una ruta `_X.` personalizada: si ya existe en `[sip-phones]` o `[rpt-internal]`, el postinstall la conserva y muestra un aviso. En ese caso hay que verificar que la ruta mantenga el encaminamiento por canal Local y llegue a `Rpt(${NODE},Pv)` de forma compatible con la instalación.

El postinstall agrega las directivas PJSIP que faltan en `modules.conf` aunque no encuentre los archivos `.so`, convierte `noload` en `load` para los módulos que el parche habilita e intenta cargarlos si Asterisk está activo. Carga primero la infraestructura y los recursos base, comprueba el estado `Running` y reintenta dependencias mientras haya progreso. Si faltan módulos requeridos, termina con error mostrando la respuesta de Asterisk, sin retirar la configuración. `res_pjsip_publish_asterisk.so` queda en `noload`: solo intercambia eventos PUBLISH entre servidores y no es necesario para extensiones SIP ni Listen Live. Así se evitan mensajes de carga fallida del módulo opcional. Si necesitas PUBLISH entre servidores, configura un Entity ID válido y cambia su directiva a `load` en `modules.conf`. No reinicia Asterisk; si está detenido, la configuración se aplica al próximo inicio. El dialplan de Listen Live requiere además los contextos `[listen-live-spy]` y `[listen-live-eagi]`, que el postinstall agrega si faltan. Los contextos existentes se conservan.

### Bind privado y firewall persistente

El instalador elige una IPv4 RFC1918 de una interfaz activa para `[transport-udp]`, prefiriendo la interfaz de la ruta predeterminada. Reemplaza binds `0.0.0.0`, preserva binds específicos y respalda los cambios; solo conserva el comodín si no encuentra una IPv4 privada. Antes de cargar PJSIP habilita e inicia `amerimon-pjsip-firewall.service`, verifica su estado `active (exited)` y bloquea UDP/5060 hacia IPv4 locales no privadas mediante firewalld existente o iptables. Si no existe ninguno, instala iptables. El bloqueo persiste tras reiniciar y cubre direcciones públicas de VPN o túneles sin modificar otros puertos.

Con iptables, comprueba `iptables -t raw -L -n -v`: las reglas están en `raw/PREROUTING`, no en la tabla `filter` mostrada por `iptables -L` sin opciones. Con firewalld, comprueba las reglas directas runtime y permanentes. Una recarga exitosa de `res_pjsip.so` no garantiza que cambie el bind de un transporte con `allow_reload=false`; puede requerir un reinicio manual de Asterisk, que interrumpe enlaces y Listen Live. El instalador no lo realiza automáticamente. Si la IP pública pertenece a un router NAT, cierra también el port-forward UDP/5060 allí. Consulta **Comprobar el firewall y el bind después de instalar** en el [manual técnico](manual-tecnico) para los comandos y diagnósticos.

## Desconexiones y fin de sesión

```mermaid
sequenceDiagram
    participant F as Frontend
    participant WS as WebSocket / Backend
    participant TCP as Flujo EAGI
    participant AST as Asterisk
    participant API as API de sesiones
    participant DB as SqLite3

    WS-->>F: Ping periódico
    F-->>WS: Pong automático del navegador
    alt El usuario detiene la escucha o falla el procesamiento local
        F->>WS: Cierra WebSocket
        F->>F: Detiene audio y resuelve inicio pendiente
        F->>API: PATCH /api/listen-live/sessions/{listenliveid}
        API->>DB: Actualiza fecha_fin
    else Se pierde la red
        WS->>WS: No llega Pong dentro del plazo
        WS->>WS: ReadMessage detecta timeout y retira el cliente
        TCP->>WS: Broadcast devuelve 0 oyentes
        WS->>TCP: Se termina el handler EAGI
        TCP->>AST: Se cierra la conexión TCP
        Note over F,DB: El frontend puede intentar PATCH, pero una caída abrupta puede impedir que llegue
    else Termina el audio de Asterisk
        TCP->>WS: Cierra los WebSockets del UUID
        WS-->>F: Close normal, motivo "audio stream ended"
        F->>F: Detiene audio y finaliza la sesión
        F->>API: PATCH /api/listen-live/sessions/{listenliveid}
        API->>DB: Actualiza fecha_fin
    end
```

El WebSocket usa heartbeat: el servidor envía Ping cada 25 segundos, espera Pong y considera desconectado al cliente tras 60 segundos sin respuesta. También hay un límite de 10 segundos para escrituras. Si una escritura de audio falla, el backend elimina y cierra ese cliente.

Cuando EAGI detecta que ya no quedan oyentes, cierra su conexión TCP; al terminar el handler se cierran los WebSockets asociados al UUID. Si el flujo nunca llegó a tener oyentes, EAGI espera hasta 15 segundos antes de terminar.

### Quién actualiza `fecha_fin`

El cierre del WebSocket **no actualiza directamente SqLite3** en el backend actual. El frontend debe conservar el `listenliveid` devuelto al crear la sesión y llamar a `PATCH /api/listen-live/sessions/{id}` con una fecha RFC3339 al detener o finalizar la reproducción. La ruta verifica que un usuario normal solo cierre su propia sesión.

Esto funciona para cierres que el navegador puede procesar. Si el proceso del navegador termina o la red desaparece abruptamente, el PATCH podría no llegar; el heartbeat detecta la conexión muerta y detiene el flujo de audio, pero no cierra por sí solo la fila de la tabla.
