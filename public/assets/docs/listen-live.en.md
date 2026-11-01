# Listen Live

Listen Live lets you listen in a browser, in real time, to audio from a SIP extension or the local node. Asterisk captures the audio; the Go backend transports it over WebSocket; the frontend processes and plays it.

## Overview

```mermaid
flowchart LR
    User[User] -->|Starts listening| Frontend[Angular frontend]
    Frontend -->|POST /api/listen-live/start<br/>ext + UUID| API[Go API / Echo]
    API -->|Originate over AMI| Asterisk[Asterisk]
    Asterisk -->|ChanSpy| Target[SIP channel or node]
    Asterisk -->|EAGI + audio_eagi.sh| TCP[TCP audio :8086]
    TCP -->|PCM SLIN16| Stream[AudioStreamManager]
    Frontend <-->|Binary WebSocket<br/>/api/listen-live/ws?uuid=...| Stream
    Frontend -->|Processed audio| Speaker[Browser audio]
    Frontend -->|PATCH fecha_fin on close| DB[(SqLite3: listenlive)]
```

The UUID correlates Asterisk's audio stream with the WebSocket. The SqLite3 listening session is identified by `listenliveid` and belongs to a user; these identifiers serve different purposes.

## Starting and routing audio

```mermaid
sequenceDiagram
    autonumber
    actor U as User
    participant F as Frontend
    participant API as Go backend
    participant DB as SqLite3
    participant AMI as Asterisk AMI
    participant AST as Asterisk / EAGI
    participant TCP as TCP listener :8086

    U->>F: Clicks "Listen"
    opt Register user session
        F->>API: POST /api/listen-live/sessions
        API->>DB: INSERT listenlive
        DB-->>API: listenliveid
        API-->>F: Session created
    end
    F->>F: Generates UUID
    F->>API: POST /api/listen-live/start {ext, uuid}
    API->>AMI: Originate Local/{ext}@listen-live-spy/n
    AMI->>AST: Runs listen-live-spy and listen-live-eagi
    API-->>F: Start accepted
    F->>API: GET /api/listen-live/ws?uuid={uuid}
    API-->>F: WebSocket upgrade
    AST->>AST: ChanSpy captures the target channel
    AST->>TCP: EAGI runs audio_eagi.sh
    TCP->>TCP: Reads UUID and PCM frames
    loop While audio is received
        TCP->>API: PCM SLIN16 audio (320 bytes / 20 ms)
        API-->>F: Binary WebSocket message
        F->>F: Decodes and plays audio
    end
```

The active backend configuration is `ListenModeEAGI`. In this mode, the `audio_eagi.sh` script first sends the UUID (36 bytes), then the audio to the TCP listener. Each typical frame is 320 bytes: PCM SLIN16, 8 kHz, approximately 20 ms.

## PJSIP and dialplan prerequisites

To listen to an extension, it must exist as a PJSIP endpoint in `pjsip.conf`, and Asterisk must have the `res_pjsip` module available. Amerimon can manage endpoints from **Admin → PJSIP extensions**; new extensions default to the `sip-phones` context. Module settings and the phone dialplan context are installed additively from `pjsip-modules-patch.conf` and `pjsip-extensions-sip-phones-patch.conf`. See **PJSIP for SIP phones** in the [technical operations manual](manual-tecnico) for installation behavior, backups, and handling existing routes.

The PJSIP patch routes calls from the phone through a Local channel to `rpt-internal` before entering `app_rpt`. It does not replace a custom `_X.` route: if one already exists in `[sip-phones]` or `[rpt-internal]`, post-install preserves it and logs a warning. In that case, verify that the route still sends the call through a Local channel and reaches `Rpt(${NODE},Pv)` in a way compatible with the installation.

Post-install adds missing PJSIP directives to `modules.conf` even when `.so` files are not found, converts `noload` to `load` for modules enabled by the patch, and attempts to load them if Asterisk is running. It loads infrastructure and base resources first, checks for `Running` status, and retries dependencies while progress continues. If required modules remain missing, it exits with an error displaying Asterisk's response without removing the configuration. `res_pjsip_publish_asterisk.so` is set to `noload`: it only exchanges PUBLISH events between servers and is not required for SIP extensions or Listen Live. This avoids failed-load messages for the optional module. If inter-server PUBLISH is needed, configure a valid Entity ID and change its directive to `load` in `modules.conf`. The installer does not restart Asterisk; if it is stopped, settings take effect at the next start. Listen Live also requires `[listen-live-spy]` and `[listen-live-eagi]`, which post-install adds if missing. Existing contexts are preserved.

### Private bind and persistent firewall

The installer selects an RFC1918 IPv4 on an active interface for `[transport-udp]`, preferring the default-route interface. It replaces `0.0.0.0` binds, preserves specific binds, and backs up changes; the wildcard is retained only when no private IPv4 is available. Before loading PJSIP, it enables and starts `amerimon-pjsip-firewall.service`, verifies `active (exited)` status, and blocks UDP/5060 to non-private local IPv4 addresses using existing firewalld or iptables. If neither exists, it installs iptables. Protection persists across reboot and covers public VPN or tunnel addresses without changing other ports.

With iptables, check `iptables -t raw -L -n -v`: rules are in `raw/PREROUTING`, not the `filter` table displayed by plain `iptables -L`. With firewalld, check runtime and permanent direct rules. A successful `res_pjsip.so` reload does not guarantee that a transport with `allow_reload=false` changes its bind; a manual Asterisk restart may be required, interrupting links and Listen Live. The installer does not restart it automatically. If the public IP belongs to a NAT router, also disable UDP/5060 port-forwarding there. See **Check the firewall and bind after installation** in the [technical operations manual](manual-tecnico) for commands and diagnostics.

## Disconnects and end of session

```mermaid
sequenceDiagram
    participant F as Frontend
    participant WS as WebSocket / Backend
    participant TCP as EAGI stream
    participant AST as Asterisk
    participant API as Sessions API
    participant DB as SqLite3

    WS-->>F: Periodic Ping
    F-->>WS: Automatic browser Pong
    alt User stops listening or local processing fails
        F->>WS: Closes WebSocket
        F->>F: Stops audio and resolves pending start
        F->>API: PATCH /api/listen-live/sessions/{listenliveid}
        API->>DB: Updates fecha_fin
    else Network is lost
        WS->>WS: No Pong arrives before the deadline
        WS->>WS: ReadMessage detects timeout and removes the client
        TCP->>WS: Broadcast returns 0 listeners
        WS->>TCP: EAGI handler ends
        TCP->>AST: TCP connection is closed
        Note over F,DB: The frontend may attempt PATCH, but an abrupt outage may prevent it from arriving
    else Asterisk audio ends
        TCP->>WS: Closes the UUID's WebSockets
        WS-->>F: Normal close, reason "audio stream ended"
        F->>F: Stops audio and ends the session
        F->>API: PATCH /api/listen-live/sessions/{listenliveid}
        API->>DB: Updates fecha_fin
    end
```

The WebSocket uses a heartbeat: the server sends a Ping every 25 seconds, waits for a Pong, and considers the client disconnected after 60 seconds without a response. Writes also have a 10-second deadline. If an audio write fails, the backend removes and closes that client.

When EAGI detects that there are no listeners left, it closes its TCP connection; when the handler ends, the WebSockets associated with the UUID are closed. If the stream never had any listeners, EAGI waits up to 15 seconds before exiting.

### Who updates `fecha_fin`

Closing the WebSocket **does not directly update SqLite3** in the current backend. The frontend must retain the `listenliveid` returned when the session is created and call `PATCH /api/listen-live/sessions/{id}` with an RFC3339 timestamp when playback stops or ends. The route verifies that a regular user can only close their own session.

This works for closures the browser can process. If the browser process exits or the network disappears abruptly, the PATCH may not arrive; the heartbeat detects the dead connection and stops the audio stream, but does not close the database row by itself.
