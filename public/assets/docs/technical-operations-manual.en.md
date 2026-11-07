# Amerimon Technical Operations Manual

## 1. Scope

Amerimon is a web application for querying AllStarLink, managing Allmon nodes, listening to live audio, and performing authorized Asterisk operations. The Angular frontend is compiled and embedded in a single Go service.

This manual describes the current source-code tree. The reference published Debian version is `1.0.4`. Clients install and update Amerimon with APT; they do not need to compile the code or run publishing tasks.

Amerimon queries Asterisk through AMI. Its post-install script prepares the modules and contexts required for Listen Live. An administrator can explicitly change the primary node's identity and access lists, and manage IAX users and PJSIP extensions from their dedicated pages. Other nodes and telephony users are not reconfigured automatically.

### EchoLink configuration

Module control and status use the local CLI (`asterisk -rx`), not AMI's `Command` action, which blacklists `module load` and `module unload`. The service needs access to the executable and Asterisk control socket. Only fixed EchoLink commands are executed, with a timeout; arbitrary browser-supplied commands are not accepted.

On entry, the current `chan_echolink.so` state is highlighted in a green box when loaded or a red box when unloaded, with explicit text. This indicates module state, not an active connection to another client. It updates after confirmed activation/deactivation or pressing **Refresh**.

**Important: incoming EchoLink connections are also subject to the node's access lists.** With `context=radio-secure`, they use the same context as AllStarLink connections. If `allowlist/<destination-node>` contains entries, only listed numbers are permitted; if empty, connections are permitted except those listed in `denylist/<destination-node>`. An allowlist match takes precedence over denylist. For EchoLink, use the connection number with the `3` prefix: EchoLink client `626066` is listed as `3626066` under the destination node. An outbound connection to a client can succeed while an incoming attempt from that client is rejected, because incoming connections evaluate these rules. The `peerip` patch does not remove or bypass this control.

Postinstall applies `echolink-peerip.awk` only to the standard `peerip` checks in `[radio-secure]`: IAX2/SIP query the IP, while EchoLink proceeds without querying that property or receiving a fictitious local IP. Existing list policy and other contexts are preserved. Changes create a `.bak-echolink-peerip` backup, preserve ownership/permissions and use the postinstall dialplan reload without restarting Asterisk. Application is idempotent and rejects duplicate sections or partial modifications. This alone does not guarantee incoming EchoLink connectivity.

Permanent activation supports both `load =` and `load =>` assignments in `modules.conf`. It switches the existing EchoLink line between `load` and `noload`, preserving formatting, comment and position. Duplicate EchoLink entries are removed, keeping only that line; an entry is added only if none exists. Other modules remain unchanged.

Under **Radio and location**, **Get coordinates from browser** requests permission and fills latitude and longitude with the administrator's device location, not the server's. HTTPS or localhost is required. Check that it matches the station; you can correct the values manually. Nothing is saved or applied automatically: press **Save** after reviewing. Permission denial or lookup failure is reported and previous values are preserved.

Optional `#tryinclude` directives targeting missing files without wildcards do not block the form and are preserved when saving. If an included file exists or its absence cannot be verified, editing is blocked to avoid ignoring options that Asterisk might be applying.

Under **More options → EchoLink configuration** (`/admin/echolink`), administrators can edit existing `elN` sections in `echolink.conf`. Files are located beside the configured `rpt.conf` (normally `/etc/asterisk`). The screen is encapsulated in `src/app/home/admin/echolink` and uses Amerimon's translation service, shared controls and administrative API. The **Connection message** field spans a full row of the form grid.

1. Select a section and review identity, radio and servers. `node` is the EchoLink number; `astnode` is saved as the current primary node number. The initial query excludes passwords: press the eye with an empty field to retrieve the saved password, or type a new one and use the eye to show or hide it. Retrieval requires administrator access, checks the file revision and disallows caching; use HTTPS. Leaving the field blank when saving preserves the existing password.
2. **Save** creates a `.amerimon.bak` backup with `0600` permissions and updates only the selected section, preserving unknown options and other sections. External file or primary-node changes require refreshing before saving. Includes and ambiguous sections require manual editing.
3. **Enable** runs `module load chan_echolink.so`; **Disable** runs `module unload -f chan_echolink.so` in both temporary and permanent modes after confirmation. Forced unloading disconnects links and may affect Asterisk stability when the module is in use; the confirmation warns about this risk. The unloaded state is checked afterwards. Selecting **Permanent change** also changes the same line to `noload` in `[modules]` of `modules.conf`. Otherwise, the file remains unchanged. Asterisk is not restarted.
4. Saving does not automatically apply configuration. For an active module, request **Apply configuration: reload module** and confirm. Unsupported reloads, rejected unloads and unexpected module states are reported as errors. A permanent file change remains saved even if the module operation fails: review the file and logs before retrying.
5. **Check ports** tests outbound TCP connections to configured servers on **5200** and checks local UDP sockets on **5198 and 5199**. It does not demonstrate Internet access or identify socket ownership.

**Public Internet reachability is not Amerimon's responsibility.** Router, NAT, firewall and provider restrictions are the administrator's responsibility. Amerimon does not open ports or change network rules. These checks do not replace connection and audio testing with an external EchoLink client. Audio telemetry mode configuration remains independent (see the audio section).

### Public password tools

The **Password tools** link in the header opens a single page (`/password-tools`) with the generator and meter. The former `/password-generator` and `/password-meter` routes redirect to this page.

The generator initially suggests a 12-character password and supports a minimum length of 8. You can combine uppercase letters, lowercase letters, numbers, and symbols; when selecting types manually, choose at least two. If none are selected, all four are used. Random selection uses the browser's cryptographic API and favors less-repeated characters to increase diversity. You can regenerate the password or copy it to the clipboard. The generated password is passed to the meter on the same page.

The meter also accepts a password of up to 128 characters and lets you show or hide it. It displays a score, complexity, additions and deductions, Shannon entropy, and recommendations about length, diversity, character groups, repetitions, and predictable patterns. The stated minimum length is 8; recommendations suggest at least 12 characters.

The displayed entropy is calculated from the observed character frequencies in the password. It describes the diversity of that sample; it is not a measure of the generator's cryptographic unpredictability or a guarantee that a password is secure. Generation and evaluation happen in the browser; these tools do not send or store the password in Amerimon's backend.

### Audio management and playback

Only administrators see the **record/playback** icon (circle and triangle) in the AllStarLink table header to open **Audio management in another window**. The **table** icon in Audio management opens the AllStarLink table. Both retain tooltips and accessible labels in Spanish and English. The original page remains open so connections and audio can be managed side by side. Allow popups for this site if blocked; the browser may choose a tab instead of a window. Credentials are not transferred: an unremembered session may require login in the new window. The audio route retains administrator authorization.

**Administration → Audio management** (`/admin/audio`) is an administrator-only option. It provides a library of WAV audio that administrators can activate or deactivate when needed. Files and settings are stored by the backend and in SQLite; signing out or closing the page does not stop scheduling. Tracks remain until an administrator explicitly deletes them.

- Upload formats: WAV, MP3, FLAC, OGG/Opus, M4A/MP4, AAC, WMA, AIFF, WebM, MKA, AMR and APE, depending on installed FFmpeg codecs. Uploads are no longer WAV-only: the server converts to 16-bit, 8000 Hz mono PCM WAV. Both input and converted output are limited to 25 MiB; conversions reaching the limit are rejected rather than saving truncated audio. One conversion runs at a time per service, with a 60-second timeout. Install `ffmpeg` before running the local installer; the Debian package declares it as a dependency. Already compatible WAV files do not require external conversion.
- `broadcast_once`: the administrator clicks the **Play on Asterisk** broadcast icon; Asterisk starts a broadcast to the primary node and its linked stations. This is a one-time action and is not repeated merely because the track remains enabled.
- `broadcast_silence`: each enabled track has its own interval, from 1 to 10080 minutes. Radio COS and receiving RPT/IAX and SIP links are checked on all monitored nodes, necessarily including the primary. Silent observations at least five seconds apart and a fresh check after preparing the audio are required before requesting playback. Activity or invalid AMI responses postpone transmission; overdue playback remains pending. The persistent claim advances the next due time before sending, in the same transaction as the playback lock, so two cycles cannot claim the same broadcast. Only one track is attempted per cycle. Activity detected at the final check cancels the claim and restores the pending due time. The reservation lasts the selected duration plus two seconds; if Asterisk rejects playback, the error is logged and the next interval is awaited.
- To distribute audio from the primary node to linked nodes, `duplex` must be nonzero (modes 1–4); Amerimon validates the effective value, including inherited template values. The **Primary node** settings screen can set `duplex` and automatically runs `module reload app_rpt.so` after saving.

**EchoLink:** `rpt playback` also depends on the EchoLink link's telemetry mode. In app_rpt 3.10.8, the default on-demand mode can prevent the link from transmitting the file even when AllStarLink links receive it. The postinstall shared by the local installer and Debian package sets `echolinkdefault = 1` in the `[node-main](!)` template, if present, and in the `rpt.conf` sections referenced by `astnode` in the `elN` sections of `echolink.conf`. Nodes inherit the template value unless they override it; nodes matching `astnode` are configured explicitly. It preserves the original as `rpt.conf.bak-echolink-telemetry` and reloads `app_rpt.so` if Asterisk is available. **Disconnect and reconnect the EchoLink links** so they adopt the setting; the installer does not disconnect them automatically. If Asterisk was not running, reload `app_rpt.so` first. This also enables other telemetry toward EchoLink; it is not limited to library files. Enabling all local telemetry or restarting Asterisk is unnecessary. Testing on 616262 confirmed audible EchoLink reception and `Tx Audio` increasing from 0 to 104 for a five-second file.

Primary resolution uses the same nodes as the monitor: without Allmon, local nodes come from `rpt.conf` and the configured AMI credentials. Allmon does not need to be created for playback on that server's local Asterisk; if the primary is not monitored or AMI configuration is incomplete, no broadcast is sent and an error is logged. On another server, “primary” means the node in that server's own `rpt.conf`, not another machine's primary.

These checks poll activity rather than locking out stations: they cannot guarantee detection of a transmission starting between queries or prevent someone speaking after playback begins. `rpt playback` does not automatically cancel audio when a new conversation starts.

Scheduling runs in the Go service, not in the browser, and continues after sign-out or an Amerimon restart. Playback state prevents overlapping broadcasts after a backend restart. By default, WAV files are stored under `astdatadir/sounds/amerimon`, as configured in `/etc/asterisk/asterisk.conf`; both local and Debian installers prepare that directory as `root:asterisk` with mode `2750`. During upgrades, installers copy existing audio files from the previous directory without deleting the originals. You can override the root with `audio.storage_dir` in `amerimon.yaml`; it must be inside a sound path Asterisk searches, readable by Asterisk, and writable by Amerimon. `audio.poll_interval` controls polling (default `5s`; accepted range is 1 to 60 seconds). The node's AMI must be permitted to execute `rpt playback`.

The `on_connection` welcome mode has been removed because `app_rpt` does not safely support playback to a single connected link. Migration 0013 changes existing tracks in that mode to `broadcast_once` without deleting their records or files. Previously stored 8-bit PCM tracks are converted to 16-bit the first time they are played.

Buttons use icons with hover descriptions and accessible labels. The headphones icon (**Listen locally**) downloads the WAV through the authenticated API for browser playback with its signal meter, including disabled or scheduled tracks. It does not broadcast through Asterisk or change scheduling. The broadcast icon (**Play on Asterisk**) retains manual transmission for enabled one-time tracks. Changing previews or leaving the screen releases the player and its temporary URL.

During a request, other library and recording actions and editable fields remain disabled until it finishes, including on failure. The initiating button displays its progress indicator and process text; upload percentage also appears there, without a duplicate outside the button. Errors and completion notices remain visible separately. An ongoing recording and background polling do not lock pause and stop controls.

The panel, meters, icon descriptions, errors, and confirmations follow the header language, including switching between Spanish and English without leaving the screen. The Angular feature is isolated in `src/app/features/audio-management`; to reuse it in another application, see its README and provide the API URL, translator, and a route guarded by administrator authorization.

### Primary node recordings

In **Administration → Audio management**, the **Primary node recordings** card captures the node/link mix. It does not use the browser microphone or transmit audio to stations. It requires the dedicated `amerimon-record-node` context using `Rpt(node,Pm)` in monitor-only phone mode, together with the `listen-live-eagi` transport, EAGI script and local listener. Spying only on the radio `rxchannel` can yield silence while linked stations speak; recording does not use that source. The installer adds the recording context without changing Listen Live's radio context or existing custom blocks. After updating the backend and dialplan, stop and restart an older capture to change its source.

- Enter a name and click **Start recording**. Only one capture can be active. Startup is confirmed after EAGI PCM arrives, not merely when AMI accepts Originate.
- Silence is omitted automatically based on PCM energy (approximately −40 dBFS RMS), retaining 200 ms before and 500 ms after detection. This is level detection, not speech recognition: noise above the threshold is also retained. The status shows **Automatic pause: waiting for audio** and resumes when audio returns; duration measures only retained audio, including these margins.
- **Pause** remains a manual pause even if audio arrives. **Continue** re-enables automatic detection. **Stop and save** finalizes the WAV and closes only the capture channel, without disconnecting stations.
- Capture continues after closing the page or signing out. The 16-bit, 8000 Hz mono PCM WAV files are saved automatically in the private `recordings` subdirectory next to the service's SQLite database (directory `0700`, files `0600`). Each recording is capped at 25 MiB, approximately 27 minutes of retained audio; reaching the limit finalizes it automatically. Recordings are not deleted based on age.
- If EAGI disconnects or Amerimon restarts, capture does not resume automatically. It is marked interrupted and received audio is retained; on startup, the WAV header is reconstructed from recovered PCM.
- **Listen** downloads the WAV through the authenticated API for browser-only playback. **Add to library** makes an independent one-time-broadcast copy without broadcasting or scheduling it automatically. You can later change mode and interval in its library card. **Delete** requires confirmation and removes only the original recording, not an existing library copy.

Additive migration `0014_audio_recordings.sql` creates `audio_recordings` without modifying existing audio or configurations. All routes require administrator authorization: `GET/POST /mon/audio/recordings`, `PATCH /mon/audio/recordings/:id` with `action: pause|resume|stop`, `GET /mon/audio/recordings/:id/file`, `DELETE /mon/audio/recordings/:id`, and `POST /mon/audio/recordings/:id/library`.

#### Selecting, previewing and saving a segment

1. On the library card, press **Configure broadcast segment**. This is also available for scheduled or disabled tracks. The manual broadcast icon opens the same dialog.
2. Select **Choose a segment**. Drag the left handle for **Start** and the right handle for **End**, or enter **Start (s)** and **End (s)** with 0.001-second precision. The handles also support keyboard adjustment and cannot cross. The highlighted bar shows the selected portion. For example, start 5 and end 20 selects 15 seconds.
3. To check without transmitting, press the **Listen to selection** headphones inside the dialog, then play in the player that appears. It includes a level meter and plays only the selected portion; **Complete audio** previews everything.
4. Press the **Save changes** disk icon to remember the bounds without broadcasting. **Play in Asterisk** saves and then requests a broadcast; it is only available when the track is enabled in manual mode. **Cancel** closes without saving pending edits.

The dialog preview is local only: it neither saves bounds nor transmits to Asterisk. Changing bounds or switching complete/segment stops and hides the previous preview; press **Listen to selection** again to load the new selection. Closing the dialog stops and releases the audio.

When a track has saved bounds, its card displays a highlighted **Segment selected** badge with start, end and duration. Saved bounds survive reloads/restarts and apply to both manual and every scheduled broadcast, without changing when the next broadcast is due.

To remove the selection, press **Reset to complete audio**, then **Save changes**. The card returns to **Complete audio**. Resetting and then cancelling does not change saved bounds.

The library file is never trimmed: the card's headphones and download still provide the complete audio. Only the dialog preview and broadcasts use the segment. Additive migration `0015_audio_segments.sql` stores `start_ms` and `end_ms` per track; existing tracks remain complete until a selection is saved. The server creates a temporary WAV for each segment broadcast and reserves playback for its duration plus two seconds; it deletes the temporary file one minute after the expected playback time while the service remains running.

#### Downloading audio

The **Download WAV** icon is available for library tracks, including disabled or scheduled ones, and finalized nonempty recordings. It downloads through the authenticated API without broadcasting or changing activation or intervals. Downloads contain the stored Asterisk-compatible WAV, not the original compressed upload; conversion does not retain that original. The button displays **Downloading...** and locks other actions until the request finishes.

#### Signal meters

- **Recording input level:** measures RMS energy of PCM received from the node, including during manual pause. It updates every 500 ms while a capture is active. It does not necessarily represent stored audio: silence is omitted and manual pause discards audio.
- **Playback level:** measures the WAV heard in the browser through Web Audio, sampled every 50 ms, for both recordings and library previews. It resets on pause or end. It does not measure Asterisk's output or physical speaker volume.
- The scale runs from −60 to 0 dBFS, with green, yellow, and red zones; 0 dBFS is the digital maximum. The visual floor is −60 dBFS. Input returns to the floor when recent samples stop arriving. No microphone permission is required. If the browser cannot start the meter, an explicit error is displayed.

## 2. Components and ports

| Component | Default address | Use |
| --- | --- | --- |
| Amerimon HTTP | `0.0.0.0:8082` in the package; `127.0.0.1:8082` in development | Embedded frontend and REST API `/mon` |
| EAGI audio | `127.0.0.1:8086` | Local audio input for Listen Live |
| Asterisk AMI | `127.0.0.1:5038` | Asterisk queries and operations |
| Angular dev server | `localhost:4200` | Development only, with `npm start` |

Port `8086` must be available when Amerimon starts. The EAGI process and AMI connection are local to the server. If Apache or Nginx is not installed, Amerimon still works directly at `http://<server-IP>:8082`; without a proxy, the `/amerimon` path and HTTPS are not available.

## 3. Technologies and data

- Backend: Go `1.26`, Echo, and WebSocket.
- Frontend: Angular 22, TypeScript 6, and Vitest.
- Database: SQLite through `modernc.org/sqlite`, a pure-Go implementation with no separate SQLite server or CGO.
- The database uses WAL, `busy_timeout`, and foreign keys.
- Schemas are applied through embedded SQL migrations. Do not edit migrations that have already been published; add a new numbered migration.
- Before migrating an existing database, Amerimon creates a `*.bak-<date>` copy and keeps the three most recent copies.

### Main tables

| Table | Purpose |
| --- | --- |
| `admin` | Administrator user and password hash |
| `users`, `listenlive` | Listen Live sessions and users |
| `asterisk_commands` | Catalog of allowed operations |
| `allstarlink_node_activity` | Most recently observed activity between nodes |
| `asterisk_connection_history` | History of observed links |
| `favoritos` | Administrator favorites |
| `enlaces_externos` | Links published and managed from the web |
| `app_settings` | Header title and subtitle |
| `terminal_credentials` | SSH credentials encrypted per destination |
| `asterisk_operation_logs` | Asterisk operations, results, and diagnostics |
| `audio_tracks` | Audio library, format, activation, and measured duration |
| `audio_broadcast_schedules` | Intervals and next/last scheduled playback |
| `audio_playback_state` | Persistent reservation preventing overlapping audio |

## 4. Debian installation

The public APT repository is `https://jjsainzc.github.io/repo-deb/` and is signed with the `hc1lnm.gpg` key. On a Raspberry Pi ARM64 or Debian AMD64:

```bash
curl -fsSL https://raw.githubusercontent.com/jjsainzc/repo-deb/main/hc1lnm.gpg \
  | sudo gpg --dearmor -o /usr/share/keyrings/hc1lnm-archive-keyring.gpg

echo "deb [signed-by=/usr/share/keyrings/hc1lnm-archive-keyring.gpg arch=$(dpkg --print-architecture)] https://jjsainzc.github.io/repo-deb/ ./" \
  | sudo tee /etc/apt/sources.list.d/amerinet.list

sudo apt update
apt-cache policy amerimon
sudo apt install amerimon
```

To install a newly published version:

```bash
sudo apt update
apt list --upgradable
sudo apt upgrade
```

Check the version and service:

```bash
dpkg-query -W -f='${Version}\n' amerimon
sudo systemctl status amerimon --no-pager
```

After a maintainer publishes a release, the APT index may take time to update. On the Raspberry Pi, confirm with `apt-cache policy amerimon` that `Candidate` shows the expected version before upgrading.

### Installed files

| Path | Contents |
| --- | --- |
| `/usr/bin/amerimon` | Go backend with embedded frontend |
| `/etc/amerimon/amerimon.env` | Address, database, and AMI credentials |
| `/etc/amerimon/amerimon.yaml` | AMI, paths, and data sources |
| `/etc/amerimon/jwt.secret` | Randomly generated JWT key, mode `0600` |
| `/etc/amerimon/allmon.ini` | Amerimon-managed node configuration, if no other active file exists |
| `/var/lib/asterisk/privatenodes.txt` | Private nodes managed from the web panel |
| `/var/lib/amerimon/amerimon.db` | Production SQLite database |
| `/var/lib/amerimon/terminal-credentials.key` | Master key for encrypting SSH credentials, mode `0600` |
| `/lib/systemd/system/amerimon.service` | systemd unit |
| `/usr/local/bin/astdb.sh` | AstDB node database updater |

`amerimon.env` and `amerimon.yaml` are installed as `config|noreplace`; an upgrade preserves existing local files.

By default, `sources.privatenodes_path` uses `/var/lib/asterisk/privatenodes.txt`. At startup, Amerimon preserves any existing file at the configured path; if it is elsewhere and the default does not exist, Amerimon creates a symbolic link to it. If the default path is selected and the file is missing or empty, Amerimon first searches for files containing data at `/etc/allmon3/privatenodes.txt` and `/var/www/html/supermon/privatenodes.txt`, linking them so existing nodes can be displayed and edited without duplication. It never replaces a file containing data or an existing link. If no previous file exists, it creates the default file. The administrator can edit, add, and delete lines from **Private Nodes**; saving preserves the shared file's permissions and group. In production, the service runs as root and also preserves the owner; in unprivileged development, the replacement may become owned by the process user, and this is recorded in the log. Each entry uses `node|callsign|description|location`.

Upgrades do not replace `/etc/amerimon/allmon.ini`: the installer creates it only if it is missing. The Allmon node editor preserves sections it does not manage and creates an `.amerimon.bak` copy before saving.

### Access control: nodes and callsigns

The dialplan contexts serve different purposes:

- **`[radio-secure]`:** controls node-to-node connections. It checks `allowlist/<destination>/<source-node>` and `denylist/<destination>/<source-node>`, using `${EXTEN}` as the destination and `${CALLERID(num)}` as the source node. The `allstar-public` patch does not modify this context.
- **`[allstar-public]`:** handles clients such as IAX Web Transceiver, Repeater Phone, and QSO One. It first authenticates the callsign through AllStarLink; invalid authentication ends the call. Here `${CALLSIGN}` is the authenticated callsign and `${NODENUM}`, taken from `${CALLERID(num)}`, is the destination node, not a source node.

After authentication, `allstar-public` applies this order:

For comparisons, it derives `CALLSIGN_BASE`: everything from the first `-` or `/` onward is removed, and the result is uppercased. For example, `HC1LNM-P`, `HC1LNM/RPT`, and `HC1LNM-P/RPT` are compared as `HC1LNM`. Store access entries using the base callsign without suffixes. The full authenticated callsign is retained in the connection identity and dialplan messages; an empty base is rejected. This normalization does not affect node numbers or `[radio-secure]`.

1. If `allowlist/${NODENUM}/${CALLSIGN_BASE}` exists, connect directly.
2. Otherwise, check `denylist/${NODENUM}/${CALLSIGN_BASE}`. If it exists, block the call.
3. If the callsign appears in neither destination-scoped list, fall back to the global `whitelist/${CALLSIGN_BASE}` and `blacklist/${CALLSIGN_BASE}` filter. Other entries in `allowlist/<destination>` do not prevent this fallback.

First-filter decisions are final: an `allowlist` match takes precedence over `denylist` and skips the global lists; a `denylist` match cannot be overridden by the global `whitelist`.

The global filter preserves the patch's policy: both lists empty means access is allowed; a `whitelist` match allows access even if also present in `blacklist`; without a `whitelist` match, a `blacklist` match blocks access. If either global list contains entries and the callsign matches neither, access is also blocked, even when only `blacklist` is populated.

In **Admin → Primary Node**, separate cards distinguish node-scoped lists (`allowlist/denylist`) from global callsign lists (`whitelist/blacklist`). Global entries have no node-number scope, and changes through AMI take effect immediately. Running `./install.sh` installs the patch in the existing `[allstar-public]` stanza and reloads the dialplan without modifying `[radio-secure]`.

### Primary node configuration

**Admin → Primary Node** provides functionality equivalent to options **1 Node number**, **2 Node Password**, **3 Node callsign**, and **7 Node access list** in `node-setup`.

- **Number:** changes the public number of the primary node identified in `rpt.conf`. This monitor requires a public primary node (4 to 6 digits, not starting with 0 or 1); private nodes are managed as connections, not as the primary. Numbers already configured and forms left open for a primary node that has changed are rejected.
- **Password:** this is the registration password obtained from the AllStarLink portal, not the Amerimon or AMI password. Only an administrator can retrieve the current password; it is hidden and can be revealed with the eye button. The active HTTP registration is preferred, falling back to the IAX registration if needed. The response is not cached; the password is not stored in browser storage or logs. Use HTTPS and a trusted device. Leaving the field unchanged or empty preserves the current password; a new password may contain 6 to 128 letters, numbers, hyphens, and underscores.
- **Callsign:** updates `idrecording = |iCALLSIGN` directly in the primary section while preserving templates. An inherited `idrecording` or one based on an audio file is not presented as an editable callsign; enter the callsign you want to use.
- **Duplex:** configures `duplex` (0–4) directly in the primary node section; if the value was inherited from a template, a local override is added without changing the template. The options are: 0, half duplex without telemetry or hang time (full duplex with `linktolink=yes`; preferred for an external multiport repeater controller; commenting out `idrecording` and `idtalkover` also suppresses IDs); 1, half duplex with telemetry and hang time, no repeated audio (preferred for a simplex node); 2, full duplex with telemetry and hang time (preferred for a repeater); 3, full duplex with telemetry and hang time, no repeated audio; 4, full duplex with telemetry and hang time, repeats audio only when the autopatch is down. Global audio broadcast to linked nodes requires a nonzero `duplex` mode (1–4).
- **Access:** adds, updates, and deletes numbers or callsigns in AstDB's `allowlist/<primary>` and `denylist/<primary>`, with an optional reason. Changes are applied immediately through AMI. With no entries, these lists impose no restrictions. An allowlist limits which nodes are accepted; blocked nodes are rejected. Reasons can contain up to 200 characters and cannot contain double quotes, backslashes, or control characters.
- **AMI access:** before the access lists, you can change the `secret` of the AMI user already configured in `ami.user`, the `[general]` `bindaddr`, and optionally add `permit`/`deny` networks to that same stanza. The secret is masked and can be revealed with the eye button. IPv4 networks accept an IP/prefix or IP/netmask; Amerimon calculates the normalized network. Existing rules are preserved only: the screen adds rules but does not delete or replace custom options. Any `deny` that covers loopback or the backend address is rejected, and explicit loopback access is added when restricting access with `permit`. If the backend AMI address cannot be resolved, ACL changes are not applied.
- This editor creates `.amerimon.bak` backups of `manager.conf`, `amerimon.yaml`, and `amerimon.env`; updates `ami.host`, `ami.user`, and the secure `ami.secret: ${AMI_SECRET}` reference in YAML; and keeps the secret value in the environment file with its existing permissions. The installed service explicitly specifies `/etc/amerimon/amerimon.env`; for custom installations, set `AMERIMON_ENV_FILE` to the environment file used by the process. When a specific `bindaddr` is used, the YAML AMI host is synchronized with that address; with `0.0.0.0` or `::`, the current host is retained. Amerimon then runs `manager reload` (without restarting Asterisk) and checks a new AMI connection. If authentication fails, it attempts to restore the files and reload the previous configuration. Do not restart Asterisk if a restoration failure is reported.

Editing changes only references to the selected node in `rpt.conf`, `rpt_http_registrations.conf`, `iax.conf`, and, if present, `savenode.conf`, `extensions.conf`, `simpleusb.conf`, `usbradio.conf`, and `voter.conf`. It also renames the corresponding section in the active Allmon file and in `/etc/allmon3/allmon3.ini`, when present. Other sections, registrations, and credentials are preserved. When renumbering, access lists are moved to the new number; a destination that already has lists is rejected.

Edited definitions must be directly in those files: `#include` files, arbitrary customizations, and numeric references in external scripts are not rewritten. If the primary section is not defined directly, saving is rejected. The process must be able to read the files, create backups, and replace them in their directories while preserving owner, group, and permissions. The web application does not elevate privileges. Asterisk paths are derived from the directory of `ami.rpt_config_path`.

Before modifying files, adjacent `*.amerimon.bak` backups are created with mode `0600`; they contain passwords and must not be published or copied to locations accessible over HTTP. The latest backup of each file is kept. If saving or migrating lists fails, Amerimon attempts to restore the previous configuration and explicitly reports any restoration failure. If a restoration failure is reported, do not restart Asterisk; check the backups first.

**Saving automatically reloads `app_rpt.so`** through AMI using `module reload app_rpt.so`; it does not restart Asterisk. The screen confirms the save only if AMI accepts the reload. If reload fails, the files have already been saved with `.amerimon.bak` backups; check the Asterisk logs and do not repeat identity changes until you confirm which configuration is active. If you changed the node number and use Allmon3, restart Allmon3 manually. Schedule changes during a maintenance window and consider their impact on node operation.

The **Clear Allowed** and **Clear Blocked** buttons remove only entries from the selected primary-node list, after confirmation, without touching the other list or other nodes. The change is immediate and cannot be undone. Clearing the allowlist removes the restriction to authorized nodes; the denylist still applies. If a deletion fails, the interface reports that clearing may be partial and the list should be reloaded.

**Export Lists** downloads both lists and their reasons to `listas-acceso-<node>.json`, querying AstDB again. The document uses `format: "amerimon-node-access"`, `version: 1`, `node` (source), and `entries` with `type`, `node`, and `reason`. It does not include passwords. **Import Lists** lets you select that JSON (up to 1 MiB and 1000 entries) and confirm the destination node. It always merges: entries not present in the file are preserved, and reasons for matching entries are updated by list and node/callsign. A file from another node can be imported, but it is applied only to the current primary node. Duplicate entries in the same list and the destination node itself are not allowed. The complete file is validated before writing, saved entries are verified, and Amerimon attempts to restore changes if an operation fails, reporting any incomplete restoration. Changes are immediate. Export requires `GET /mon/primary-node/access/export`; import requires `POST /mon/primary-node/access/import` with `primary_node` and `document`; both are restricted to administrators.

The API requires a JWT with the administrator role for `GET/PUT /mon/primary-node`, `GET/PUT /mon/primary-node/ami`, `GET/PUT/DELETE /mon/primary-node/access`, and `POST /mon/primary-node/access/clear`. Clearing requires `primary_node`, `type` (`allowlist` or `denylist`), and `confirmation` (`VACIAR allowlist` or `VACIAR denylist`). These feature tests use temporary files and simulated AMI; they do not modify the real node configuration.

## 5. systemd service

The unit is `amerimon.service`. It starts automatically when the package is installed and restarts if it fails.

```bash
sudo systemctl start amerimon
sudo systemctl stop amerimon
sudo systemctl restart amerimon
sudo systemctl status amerimon --no-pager
sudo journalctl -u amerimon -f
```

The service receives `jwt.secret` as a systemd credential and runs the binary as root, which is required for system paths and the configured EAGI integration.

### Environment variables

`/etc/amerimon/amerimon.env` supports:

| Variable | Package default | Description |
| --- | --- | --- |
| `AMERIMON_ADDR` | `0.0.0.0:8082` | HTTP interface and port |
| `AMERIMON_DB` | `/var/lib/amerimon/amerimon.db` | SQLite file |
| `AMI_USER` | Empty until detected | AMI user |
| `AMI_SECRET` | Empty until detected | AMI secret |
| `AMERIMON_JWT_SECRET_FILE` | `/etc/amerimon/jwt.secret` | Alternate path to the JWT secret |

After editing the configuration:

```bash
sudo systemctl restart amerimon
sudo journalctl -u amerimon -n 100 --no-pager
```

Do not publish `amerimon.env`, `jwt.secret`, database copies, or AMI passwords.

## 6. Asterisk and node configuration

### Global AMI

The `/etc/amerimon/amerimon.yaml` file defines the global AMI connection:

```yaml
ami:
  host: 127.0.0.1
  port: "5038"
  user: ${AMI_USER}
  secret: ${AMI_SECRET}
  listen_port: 8086
  rpt_config_path: /etc/asterisk/rpt.conf
  pjsip_config_path: /etc/asterisk/pjsip.conf
  activity_poll_interval: 5s
```

On Debian installations, `postinstall.sh` attempts to read the first AMI user other than `[general]` from `/etc/asterisk/manager.conf`. It adds `AMI_USER` and `AMI_SECRET` only if they are not already defined in `amerimon.env`. If it cannot read them, Amerimon is still installed, but AMI queries and operations will not work until these variables are set. An administrator can also update the configured AMI user's secret and bind address from **Admin → Primary Node**; this preserves the file and its custom settings, creates a backup, updates `amerimon.env`, and runs `manager reload` without restarting Asterisk.

Amerimon does not create Asterisk users. The existing AMI user must have the permissions required for the operations in use, including command execution and `Originate` for Listen Live. `permit`/`deny` networks added from the web are additive and do not replace existing custom ACLs.

If `/etc/asterisk` exists, the Listen Live step saves the `modules.conf` state it finds as `modules.conf.bak-listen-live` and enables the modules present among `res_speech.so`, `res_agi.so`, `app_chanspy.so`, `app_mixmonitor.so`, `app_verbose.so`, `func_config.so`, `func_strings.so`, `func_logic.so`, and `func_cut.so`. If the PJSIP step has already modified the file, this intermediate backup includes the PJSIP changes; the original before those changes is in `modules.conf.bak-pjsip`. `app_verbose.so` provides the context's `Verbose` and `Log` applications; if it is not loaded, Asterisk ends the call before reaching ChanSpy. The script converts a `noload` directive for these modules to `load`, or appends `load => ...`. If `extensions.conf` exists, it saves the state before the Listen Live dialplan changes as `extensions.conf.bak-listen-live` and adds the `[listen-live-spy]`, `[listen-live-eagi]`, and `[amerimon-listen-node]` contexts if missing. It migrates only recognized legacy Amerimon `nodo` blocks (SimpleUSB or Local/pseudo), replacing them with a single redirect to the new context; custom blocks are preserved and generate a warning. Before migration, it also saves `extensions.conf.bak-listen-live-rxchannel`.

If Asterisk is already running, the hook attempts to load these modules and run `dialplan reload`; it does not restart the Asterisk service. If a module does not exist in that distribution, a warning is logged and the Amerimon installation continues. The `.bak-listen-live` copies can be used to review or manually restore the previous state.

### PJSIP for SIP phones

Post-install enables and starts `amerimon-pjsip-firewall.service` with `systemctl enable --now` and checks that it is active before configuring PJSIP. Its normal state is `active (exited)` because it is a `oneshot` unit with `RemainAfterExit=yes`. If already active, the updated script verifies the rules again without restarting the unit or Asterisk. Startup or status-check failures abort configuration with a diagnostic directing the administrator to the service journal.

The post-install script also merges `packaging/pjsip-modules-patch.conf` and `packaging/pjsip-extensions-sip-phones-patch.conf`. If either configuration file is missing, it creates `modules.conf` with `[modules]` and `autoload=yes`, or creates `extensions.conf`. It adds every missing module directive regardless of whether the `.so` file is present. For modules enabled by the patch, it converts `noload` to `load` and removes duplicates. Other modules and custom settings are preserved, and the patch's `noload` directives are added when no directive already exists for those modules. In `pjsip.conf`, it adds `[transport-udp]` (`type=transport`, `protocol=udp`) bound to a private IPv4 on an active interface. It prefers an RFC1918 address on the default-route interface, otherwise the first address in `10/8`, `172.16/12`, or `192.168/16`, including VPNs and tunnels. It replaces existing `0.0.0.0` binds, preserving the port, and adds a missing bind; specific binds and commented examples are preserved. Changed files are backed up as `pjsip.conf.bak-pjsip`. It uses `0.0.0.0` only if no private IPv4 is available, and aborts without enabling PJSIP if no usable address is found. This step does not download Asterisk packages.

Before loading PJSIP, `pjsip-network.sh protect` verifies rules blocking UDP/5060 to non-private local IPv4 addresses. It reuses running firewalld, otherwise installed iptables. If only stopped firewalld exists, it enables it; if neither exists, it installs iptables through APT. It never flushes rules or changes default policies or other ports. The `AMERIMON-PJSIP` chain in `raw/PREROUTING` permits RFC1918, loopback, and link-local destinations and drops other `LOCAL` destinations before established-connection ACCEPT rules. This covers new public addresses on physical interfaces, VPNs, and tunnels without a fixed IP list and does not affect forwarded traffic. Firewalld receives runtime and permanent rules. `amerimon-pjsip-firewall.service` reapplies and verifies protection at boot, after networking and before Asterisk, which requires the service; failure blocks configuration or dependent startup. Removing Amerimon disables the service without deleting existing firewall rules. If the public IP belongs to a NAT router, also disable UDP/5060 port-forwarding there: the server sees the translated private destination, and a private bind cannot prevent it.

#### Check the firewall and bind after installation

Run on the server:

```bash
systemctl status amerimon-pjsip-firewall.service --no-pager
journalctl -u amerimon-pjsip-firewall.service -b --no-pager
iptables -t raw -L -n -v
iptables -t raw -S AMERIMON-PJSIP
asterisk -rx 'pjsip show transport transport-udp'
netstat -uln
```

With iptables, expect a UDP/5060 jump from `PREROUTING` to `AMERIMON-PJSIP`, private-destination `RETURN` rules, and a `DROP` for other local destinations. `DROP` counters show discarded traffic, not necessarily an attack. `iptables -L -v` without `-t raw` displays the `filter` table, which may be empty even when protection is working. With firewalld, use `firewall-cmd --direct --get-all-rules` and `firewall-cmd --permanent --direct --get-all-rules` instead.

The service should be enabled and `active (exited)`. Older installations may show `inactive (dead)` because they applied rules by running the script directly; this does not remove existing rules. To activate and verify the unit without restarting Asterisk:

```bash
systemctl enable --now amerimon-pjsip-firewall.service
systemctl status amerimon-pjsip-firewall.service --no-pager
```

Compare the bind in `pjsip.conf` with the active Asterisk transport and the `netstat` socket: these are separate checks. If Asterisk reports `allow_reload : false`, `Module 'res_pjsip.so' reloaded successfully` does not confirm that an existing transport changed its bind. Amerimon does not automatically enable `allow_reload` or restart Asterisk. If `0.0.0.0:5060` remains active, first verify the public-port block, then apply the private bind through a manual Asterisk restart during a maintenance window; restarting interrupts connections and listening sessions. This protection covers UDP/IPv4 on port 5060, not other transports, ports, or IPv6.

The `[sip-phones]` context routes PJSIP calls through a Local channel before passing them to `app_rpt`, avoiding direct PJSIP dialplan handling that can cause Asterisk to fail:

```ini
[sip-phones]
exten => _X.,1,Ringing()
exten => _X.,2,Answer(3000)
exten => _X.,3,Set(__NODE=${EXTEN})
exten => _X.,4,Set(__AMERIMON_SIP_ENDPOINT=${CHANNEL(endpoint)})
exten => _X.,5,Set(TALK_DETECT(set)=250)
exten => _X.,6,Dial(Local/${NODE}@rpt-internal,,,b(sip-bridge^addheader^1))
exten => _X.,7,Hangup()

[rpt-internal]
exten => _X.,1,GotoIf($["${AMERIMON_SIP_ENDPOINT}" = ""]?legacy)
exten => _X.,2,Rpt(${NODE},Pv,SIP/${AMERIMON_SIP_ENDPOINT})
exten => _X.,3,Hangup()
exten => _X.,4(legacy),Rpt(${NODE},Pv)
exten => _X.,5,Hangup()
```

If either context already exists without an `_X.` route, the installer adds the patch routes inside that context. The two exact routes from the previous patch are migrated to identify SIP RX. Custom `_X.` routes are left unchanged without duplicate priorities, with a warning if manual integration is needed. Before changing existing files, it creates `modules.conf.bak-pjsip` and/or `extensions.conf.bak-pjsip` if those backups do not already exist. Newly created files are not backed up.

If Asterisk is running, post-install loads Sorcery, PJPROJECT, and RTP first, followed by base PJSIP resources and finally the channel driver. Only modules with status `Running`, not `Not Running`, count as active. It automatically retries pending modules while other loads resolve dependencies, without a two-pass limit. If progress stops with required modules missing, it exits with an error and displays Asterisk's response; it does not report successful activation or remove configuration or backups. It verifies `[transport-udp]` in Asterisk and confirms the UDP socket for the configured address and port with `netstat`. If the bind changed or either check fails, it runs `module reload res_pjsip.so` and checks again. If the expected socket remains absent, it reports Asterisk's response without restarting the service; some transports require a manual restart to change their bind. The Debian package includes `net-tools`, `iproute2`, and the alternative `iptables | firewalld`. `res_pjsip_publish_asterisk.so` is disabled with `noload`: it exchanges PUBLISH events between servers and is not required for SIP extensions, PJSIP phones, or Listen Live. This prevents failed-load messages for the optional module during installation and Asterisk startup. If your installation needs inter-server PUBLISH, first configure a valid Asterisk Entity ID, then change its `noload` directive to `load` in `modules.conf`; diagnose any load failure with `asterisk -rx 'core show settings'` and the Asterisk log. If Asterisk is stopped, persistent directives take effect at the next start. Dialplan directives are added during post-install; review any warnings when a custom route exists.

### Allmon node selection and syntax

`allmon.ini` is searched in this order:

1. `sources.allmon_ini_path`, if explicitly configured.
2. `/etc/allmon3/allmon.ini`.
3. `/var/www/html/supermon/allmon.ini`.
4. `sources.allmon_ini_own_path`; defaults to `/etc/amerimon/allmon.ini`.

Local nodes are obtained from `/etc/asterisk/rpt.conf` (or `ami.rpt_config_path`), including the primary identified by `<node> = radio@127.0.0.1/<node>,NONE` in `[nodes]`. Nodes in `privatenodes.txt` and those classified as private (four digits) are excluded as servers: they do not create tables or server-selector options. Their connections still appear in the table for the node to which they are connected, whether the primary or another server. The primary is not selected by order: if several non-private entries match the loopback rule, an ambiguity error is logged. Local nodes use Amerimon's global AMI credentials. Allmon entries pointing to loopback or the global AMI host do not replace these nodes or their credentials. Remote server instances retain their Allmon configuration. This selection is shared by the table, activity polling, and node operations.

In the AllStarLink monitor, the primary node's table appears first; the rest are sorted by number. Each table header shows `number - description - IP/hostname`; the primary keeps its **Primary node** label, and remote tables do not have a **Remote node** prefix. The description comes from `astdb.txt`, and the address comes from the global AMI host for local nodes or Allmon's `host` field for remote nodes (without the port). The configured IP or hostname is displayed without DNS lookups; missing data is omitted, even when the server is disconnected.

Under **Update every → Real time**, the browser opens `/mon/allstarlink/activity/live` on the same server and port as Amerimon's other WebSockets. A shared AMI poller checks local COS (`NodeStat`, `SignalOnInput`) and each link's active reception (`SawStat`, RX flag) every 250 ms; queries for a server do not overlap or block other servers. The WebSocket publishes state every 250 ms. If it cannot connect, the view polls the same state over HTTP every 500 ms. The state also includes connected nodes observed by `SawStat`; when it detects a change, the view refreshes only the affected server's table to show new IAX/IAXRpt connections or remove disconnected ones. Row highlights and blinking green header indicators turn off on the next update detecting inactive COS/RX, without waiting for history to expire. Configured nodes with active COS are also highlighted in tables where they are connected. Latency includes AMI and network response times; states not refreshed expire after three seconds. Last-activity history is maintained separately. Selecting another interval stops monitoring in the view.

The real-time poller also queries `core show channels concise` and publishes PJSIP endpoints with channels in the `Up` state in `sip_connections`, over both WebSocket and HTTP. Starting or ending a SIP call refreshes the affected server's table even when its IAX links have not changed. A registered contact without an active call does not trigger a SIP connection. If the channel query fails, the error is logged and that server's SIP state is omitted from the update rather than interpreted as a disconnect.

SIP row highlights and the server indicator use `TALK_DETECT` events `ChannelTalkingStart`/`ChannelTalkingStop`, not the `Up` state, silence RTP packets or `SawStat` PTT (which does not reflect phone VOX in app_rpt 3.10.8). The patch enables `TALK_DETECT(set)=250` on the PJSIP channel before `Dial(Local/...)`, publishing voice stop after 250 ms of silence with Asterisk's default voice threshold. One shared AMI session per server receives channel events, identifies the PJSIP endpoint and destination node (`Exten`), and publishes `SIP/<endpoint>` activity over HTTP and WebSocket. Indicators clear on silence, Hangup, connection expiry or loss of the AMI session. Concurrent calls are tracked by channel and Uniqueid.

Installation and upgrades migrate only exact routes from previous patches (anonymous or named endpoint), backing up to `extensions.conf.bak-pjsip`. Custom routes are preserved with a warning if the detector is missing. The module patch loads `func_talkdetect.so`; AMI needs `call` read permission for voice and Hangup events. Custom AMI permissions are not changed automatically. The Local channel, Pv mode and RTP configuration are retained. After applying the dialplan and loading the module, reload with `asterisk -rx 'dialplan reload'`, restart only Amerimon if its backend was updated, and reconnect the phone. Verify that speaking and silence add and remove `SIP/<endpoint>` activity in `/mon/allstarlink/activity?recent=true`. Calls opened before the detector was enabled must reconnect; after AMI reconnects, detection resumes on the next voice start.

In real-time mode, the last active link stays first in its table after audio stops, without retaining the highlight or blinking indicator. New activity takes priority; the selected sort field breaks ties. Leaving real-time mode restores the selected sort order.

Navigating away closes the WebSocket and stops the view's timers, but preserves the update option during navigation. Returning to the monitor automatically opens a new connection if **Real time** was selected.

Only the node number is bold; the description and IP/hostname use normal weight, for both primary and remote nodes.

When a valid `allmon.ini` with remote nodes exists, those instances provide each remote server's AMI connection. A typical entry is:

```ini
[616260]
system=Nodes
host=127.0.0.1
port=5038
user=ami_user
passwd=CHANGE_THIS_SECRET
hideNodeURL=no
menu=yes
```

- The section name is the local node number, up to seven digits.
- `system=Nodes` identifies a node instance; `menu=no` excludes it.
- `host` and `user` are required; `port` is optional and defaults to `5038`.
- The parser accepts `host:port` in `host` or a separate port.
- If the instance password is empty and its host/port/user match global AMI, the global secret is reused.
- The Allmon editor validates up to 50 nodes, creates an `allmon.ini.amerimon.bak` copy, and rewrites only node sections.
- The **Tracking** checkbox under **Administration → Allmon Nodes** lets you exclude a node from the monitor, its activity queries, and its operations without deleting it. Amerimon stores the choice as `amerimon_tracking=disabled` in its stanza; when re-enabled, only that setting is removed and other configuration is preserved.
- Under **Administration → Favorites**, a single **Search** field queries AstDB by node number, callsign, or description. One match selects the node automatically; if there are several, you can choose the destination. The favorite stores the selected number.
- The same search includes EchoLink when the primary server's AMI confirms that `chan_echolink.so` is loaded and `echolink dbdump` returns valid `number|callsign|IP` records. You can search by the original number, the connection number with the `3` prefix, or callsign. For example, `996546|W2LRC-R|165.227.99.201` is offered and saved as `3996546`, with the suggested description `W2LRC-R EchoLink 165.227.99.201`. If the module is not loaded or there are no records, no EchoLink destinations are added; query failures are logged and AstDB search remains available. The module is not loaded or reconfigured automatically.

If no Allmon instances are configured, local nodes are read from `rpt_config_path` (normally `/etc/asterisk/rpt.conf`) and all use global AMI.

The Allmon configuration can be viewed and edited under **Administration → Allmon Nodes**. Changes to `host`, `port`, `user`, or `passwd` do not require an Asterisk restart; Amerimon reads the file again on its next operation.

### Connections and verification

Built-in actions use `rpt cmd` over AMI:

| Action | Temporary `ilink` code | Permanent code |
| --- | ---: | ---: |
| Transceive / Connect | `3` | `13` |
| Monitor | `2` | `12` |
| Local Monitor | `8` | `18` |
| Disconnect | `11` | Not applicable |

For temporary connections, Amerimon queries `XStat` until the link is confirmed as `ESTABLISHED`. The default timeout is 15 seconds and can be configured from 1 to 300 seconds in the request. Permanent connections do not wait for this automatic confirmation.

#### Already-linked node: changing mode or permanence

In **Connect remote node**, the selected server, remote node, mode (**Transceive**, **Monitor**, or **Local Monitor**), and **Permanent connection** checkbox define the requested connection settings. After confirmation, Amerimon queries the live state through AMI:

- **Same mode and same verifiable permanence:** a notice reports that the node is already connected or connecting with those settings, without disconnecting it or sending another connection command. The API returns HTTP `409`.
- **Different mode or permanence:** only that remote node's direct link on the selected server is disconnected using `ilink 11`, which also removes permanent links. Other links and servers are not disconnected.
- **Previous permanence cannot be verified:** the confirmation notice explains that the link will be disconnected and reconnected using the requested settings. A connection without a record is not assumed to be temporary.

For example, changing a **permanent Transceive** link to **temporary Monitor** first removes the old link and then sends `ilink 2`; changing only permanence also requires reconnection. Amerimon waits up to 15 seconds for `XStat` to confirm that the old link has disappeared, validates the complete topology again, and only then sends the new command. If disconnection, querying, or absence verification fails, the new connection is not started. It also refuses to connect if another route would create a loop afterward. A removed link is not automatically restored if reconnection fails; review **Asterisk Logs** before retrying.

`XStat` reports mode, but not permanence. Amerimon compares the current state with the latest compatible link command retained in **Asterisk Logs**. A later disconnection invalidates that reference. Connections created outside Amerimon or without a compatible record are treated as having unknown permanence; external permanence changes without a mode change cannot be detected through `XStat`.

This behavior applies to both `POST /mon/asterisk/commands` and `POST /mon/asterisk/connections`. The explicit **Disconnect all before connecting** option remains different: when selected, it removes all RPT links from the selected server first, not just the requested node. Leave it unchecked to change only the settings of an existing link.

#### Preventing loops among visible nodes

All nodes are treated as peers: no hub is required and there is no hop limit. Before starting a link, Amerimon queries the `XStat` of tracked local and remote nodes, as well as the requested source, through AMI. It builds an undirected graph from their established or connecting `Conn:` links and the reachability announced in `LinkedNodes:`. `LinkedNodes: <NONE>` is Asterisk's marker for an empty list, not an invalid route; `Conn:` links in the same response are still checked. An existing direct link at the source is handled according to the connection-setting rules above; if removed, the graph is queried again. If source and destination still belong to the same component, the new link is rejected with HTTP `409`, showing the visible or pending route that prevents it. Self-connections and mirrored connections visible only from the other endpoint are also rejected. `LinkedNodes:` routes indicate reachability, not necessarily every physical hop. `XStat` is requested through AMI (`Action: RptStatus`, `Command: XStat`, `Node: <node>`), not through the `rpt xstat` CLI command.

On the server, this validation applies to Connect, Monitor, and Local Monitor requests to `POST /mon/asterisk/commands`, both temporary and permanent, and to every mode of `POST /mon/asterisk/connections`. Validation rejections are recorded in **Asterisk Logs**. The monitor displays the reason and removes the pending connection from the interface. SQLite history is not treated as proof of an active connection: AMI is queried again before the command is sent.

If any node in scope cannot be queried, or its response does not contain an interpretable link list, that response is omitted and the available information from other nodes and pending reservations is checked. An incomplete topology does not itself block the new command or display a validation error; diagnostics remain in the server log. If the topology inventory cannot be loaded, at least the selected source is queried. Self-links and visible or pending routes that are detected are still rejected with HTTP `409`. **Loop prevention cannot be guaranteed when nodes are unreachable.** Failure of the requested AMI operation is still displayed as an error. Nodes excluded through **Tracking** are not queried; routes to them announced by other nodes can still be detected. Disconnect does not require a complete topology. With `disconnect_before_connect`, the source disconnect is confirmed first and the available topology is then checked again; a self-link request is rejected before anything is disconnected. Verification that the old link has disconnected remains mandatory when changing its settings.

In the monitor, node-query failures, including the primary node, do not display the unreachable node's error message. Its group retains the background retry status every 5 seconds without blocking other servers. General configuration errors and failures of user-requested actions remain visible.

Amerimon serializes link operations within this process; a concurrent request receives HTTP `409` and must be retried. After AMI accepts a link, Amerimon keeps an in-memory reservation for the connection timeout (15 seconds by default) until it is observed in `XStat`, preventing another request from closing a loop while the link is being established. Reservations do not survive a restart; visible connections are queried again. This protection cannot guarantee hidden routes, simultaneous changes made outside Amerimon, or future Asterisk reconnections of permanent links after the reservation expires.

Listen Live receives audio only from the **primary node configured for Asterisk**, identified by the `<node> = radio@127.0.0.1/<node>,NONE` entry in `rpt.conf`. For the `ext=nodo` request, Amerimon originates `Local/<node>@amerimon-listen-node/n`. The context gets the effective `rxchannel` value through `AST_CONFIG(...,rxchannel,-1)`, respecting templates and includes processed by Asterisk, and runs `ChanSpy` in listen-only mode. The path configured in `ami.rpt_config_path` is passed as the inherited AMI variable `__AMERIMON_RPT_CONFIG`.

SimpleUSB is not enforced: for example, `SimpleUSB/616265` and `Radio/616265` are preserved; `Local/pseudo` is normalized to `Local/pseudo@default`, while `Local/pseudo@other-context/n` is normalized to `Local/pseudo@other-context` to find the active channel prefix. Whisper and barge are not enabled. If `rxchannel` is missing, the dialplan logs an error and hangs up rather than spying on an arbitrary channel. This does not prove audio compatibility with every technology; each device must be tested with real traffic.

The installer adds `[amerimon-listen-node]` if missing and reloads the dialplan without restarting Asterisk. The `[listen-live-spy]` context remains for listening to PJSIP extensions. After an upgrade, stop and start a new listening session: an existing call does not switch applications when the dialplan is reloaded. If the primary node entry is missing, the API returns a configuration error.

If an intermediate version was installed that generated the `amerimon-listen-node` context with `Rpt(...,Pm)`, the installer recognizes that exact block and migrates it to read `rxchannel`. It does not replace customized variants of that context.

To test each Debian 12/13 installation after upgrading:

1. Stop the previous listening session and check `asterisk -rx "dialplan show amerimon-listen-node"`.
2. Start Listen Live with known traffic on the node. `asterisk -rx "core show channels concise"` should show `ChanSpy` with the actual receive channel, not an enforced technology.
3. Check `journalctl -u amerimon -f`: `primer audio recibido` should appear. Also confirm that the browser plays the traffic; receiving blocks alone does not guarantee audible content.
4. Stop listening and confirm the summary reports received blocks/bytes, `hangup confirmed`, and no Local channels for that session.

Remote Allmon nodes are not selected. The EAGI port `8086` remains bound to loopback.

### Diagnosing and cleaning up orphaned channels

The package installs `/usr/local/bin/limpiar_canales_listen_live.sh`. The default `--check` mode only lists `Local/...@listen-live-spy` and `Local/...@amerimon-listen-node` channels, and `audio_eagi.sh`, `cat`, and `nc` processes associated with the listener:

```bash
sudo limpiar_canales_listen_live.sh --check
```

To request hangup of the channels and terminate those EAGI processes:

```bash
sudo limpiar_canales_listen_live.sh --clean
```

Cleanup interrupts all active Listen Live sessions and requires entering `LIMPIAR`. The script does not kill the Asterisk process or its process group. Zombie processes cannot be killed directly; Asterisk must reap them.

## 7. Database, migrations, and backups

In production, the database is `/var/lib/amerimon/amerimon.db`; in development with `make run`, it is `/tmp/amerimon-dev.db`. Changing `AMERIMON_DB` selects another file.

Migrations are applied when the database is opened. If migrations have already been applied, a date-suffixed backup is created before applying new ones, and the three most recent copies are kept.

For a consistent manual copy:

```bash
sudo systemctl stop amerimon
sudo cp -a /var/lib/amerimon/amerimon.db \
  "/var/lib/amerimon/amerimon.db.manual-$(date +%Y%m%d-%H%M%S)"
sudo systemctl start amerimon
```

To restore, stop the service, keep a copy of the current database, replace the file with the backup, and start the service again. Preserve root ownership and permissions, and check the status with `journalctl`.

## 8. Administration and endpoints

On first access, click **Admin** and set the username and password. The username must contain 3 to 32 characters allowed by the application; the password must contain at least 8 characters and is stored with bcrypt. Administrative endpoints require an administrator-role JWT.

### Warning and confirmation preferences

Warnings and confirmations include **Do not show this type of notice again**. Preferences are saved per notice type, not globally: hiding connection confirmations does not hide disconnection confirmations or the confirmation for disconnecting all links before connecting. Maintenance actions, deleting favorites and external links, and saving, importing, and clearing primary-node lists have independent preferences; allowlist and denylist clearing are also separate.

For confirmations, checking the box saves the preference only when the action is confirmed; cancelling does not save it. Subsequent requests of that type, initiated by the user, execute without asking for confirmation, including destructive actions. For maintenance, after the first valid typed confirmation, the backend confirmation contract is preserved even when the dialog is no longer shown. Hiding notices does not trigger actions by itself or remove server-side validation.

Preferences are stored in Local Storage under `amerimon.notice-preferences.v1`, for the same origin and browser profile. They survive reloads, sign-out, and subsequent sign-in; they are not synchronized with other browsers or devices. **Error messages and failed-operation diagnostics are always displayed and never include a suppression checkbox.** If the browser prevents reading or saving preferences, an error is displayed; failed writes do not hide the notice in memory.

Temporarily, **Admin → Customize Header → Reset warnings and confirmations** restores all hidden notice types. It does not save header fields or modify credentials, sessions, nodes, or server configuration. Reset reports success only if the browser preference is successfully removed.

### SSH terminal

The terminal is available to administrators and connects from the Amerimon backend to the specified SSH host through the application's WebSocket. For the same server, use `127.0.0.1` if SSH is active; for another server, enter its private IP or DNS name and the port on which `sshd` listens. The destination only needs to be reachable from the backend network: **do not expose SSH to the Internet**.

1. Enter the host, port, and remote username, then select **Password** or **Public key**. For public-key authentication, select the private-key file on your device; do not enter a server-side path. The matching public key must be authorized for the remote user. If the private key is encrypted, enter its passphrase; use the eye button to show or hide it.
2. To remember credentials, select the private key first (wait for **Private key ready**), enter its passphrase if applicable, and click **Save settings and credentials**. A confirmation message indicates whether they were saved. The SSH password can also be shown or hidden with its eye button.
3. The destination settings are stored in the browser; passwords, private keys, and passphrases are not. When credentials are saved, Amerimon encrypts them with AES-GCM and stores them in the `terminal_credentials` table, scoped by host, port, username, and authentication method. The master key is stored separately from SQLite at `/var/lib/amerimon/terminal-credentials.key` with mode `0600`. On connection, the backend retrieves credentials for that destination; if none exist, it prompts for the password or private-key selection. Sign in as administrator and protect web access with HTTPS to use this feature.
4. An unknown SSH host key displays its fingerprint and requires explicit confirmation. Verify the fingerprint with the server administrator before accepting it; if a known key changes, the connection is rejected.
5. **Disconnect** closes the session and clears the terminal screen and scrollback. Typing `exit` in the remote shell also closes the session normally.

The `sudo amerimon-reset-password` command deletes both the administrator account and saved SSH credentials; save them again afterward. Do not remove or replace the master key while you need to keep existing encrypted credentials: they cannot be decrypted without the original key.

### IAX users and PJSIP extensions

Open **Admin → More options → IAX users / PJSIP extensions**, at `/admin/iax-users` and `/admin/pjsip-users`. These are administrative pages, not modal dialogs, and load their lists when opened. Titles, labels, buttons, empty states, loading messages, generic errors, and deletion confirmations follow the Spanish or English language selected in the header, including when switching languages with a form open. Configuration values are not translated. Diagnostics returned by the backend are preserved verbatim so technical information is not lost.

1. Click **New IAX user** or **New PJSIP extension**, or select a list entry to edit it.
2. IAX exposes **Name (section)**, **Context**, **Secret (password)**, and **Authentication**. PJSIP exposes **Extension number (ID)**, **Context**, **Password**, **Caller ID**, and **Maximum contacts**.
3. The name or ID is locked while editing; renaming is not supported by these forms. Context and secret or password are required; maximum contacts must be a positive integer. Identifiers accept 1 to 80 ASCII letters, numbers, periods, hyphens, and underscores; IAX reserves `general`. The backend rejects newlines, null characters, and the `;` and `#` comment delimiters in submitted values.
4. **Save** remains disabled until values change and the form is valid; restoring the original values disables it again. Saving applies changes and reloads the list. If the IAX name does not match the recognized amateur radio callsign format, a confirmation is requested because the section may belong to system configuration. Recognized callsigns and PJSIP extensions are saved directly, without this warning. **Cancel** closes the editor without saving. **Delete** is available while editing and requests confirmation containing the entry's identifier; names such as `hc1lnm-m`, `hc1lnm-p`, and other usernames are supported. The legacy `iaxrpt` section remains visible but cannot be deleted.
5. The save confirmation for sections that do not look like callsigns only applies to IAX. You can select **Do not show this type of notice again**; the preference is stored in this browser and future saves of this type proceed without asking. It can also be reset from **Admin → Customize Header → Reset warnings and confirmations**.
6. If an operation fails, the error is displayed and the edited data is retained for correction or retry. **Reload list** retries the query without saving the form. In PJSIP, **Cancel** discards edits and refreshes the list; a successful save also returns to the refreshed list, both when creating and editing. Saving and deleting are disabled while another modification is in progress.

#### Files and reloads

| Module | File | AMI reload |
| --- | --- | --- |
| IAX | `iax.conf` in the directory containing `ami.rpt_config_path`; defaults to `/etc/asterisk/iax.conf`. With `[iaxclient](!)`, new users go into the file specified by its own `#tryinclude` | `iax2 reload` when creating, editing, or deleting users |
| PJSIP | `ami.pjsip_config_path`; defaults to `/etc/asterisk/pjsip.conf`. If empty, it is located alongside `ami.rpt_config_path` | `module reload res_pjsip.so` when creating, editing, or deleting extensions |

The service user needs to read and write the files and create files in their directory. Before saving, an `<file>.amerimon.bak` backup is created or replaced with mode `0600`; configuration replacement is atomic, resolves symbolic links, and preserves owner, group, and permissions. If backup creation fails, the configuration is not changed. Include these files and backups, which contain passwords, in a restricted-access and backup policy; they are independent of SQLite.

#### IAX users with a template and included file

Amerimon checks for the `[iaxclient](!)` template in `iax.conf`. The `!` marks a template, not a user. When present, it looks for exactly one `#tryinclude` **inside that same stanza**, stopping at the next section header. Comments and includes in other stanzas do not determine the destination. The filename is not fixed:

```ini
[iaxclient](!)
type = friend
context = iax-client
auth = md5
host = dynamic
#tryinclude custom/iax/iaxclient-users.conf
```

In this example, new users are written to `/etc/asterisk/custom/iax/iaxclient-users.conf`. Relative paths are resolved from the directory containing `iax.conf`; absolute paths are respected. If the file or its directories are missing, they are created automatically when saving the first user; absence is not treated as an error. A new file uses mode `0640`. Existing content is preserved and backed up before modifications.

Each new user inherits from `iaxclient` and contains only user-specific settings:

```ini
[hc1lnm-m](iaxclient)
    user = hc1lnm-m
    secret = USER_PASSWORD
    callerid = "HC1LNM-M"
```

Names must be unique across `iax.conf` and the included file. Suffixes such as `-m` or `-p` can distinguish clients. Existing users are not moved: main-file users stay there, while users inheriting from `iaxclient` in the included file appear in the same list for password editing or deletion. Editing preserves their other settings.

Without `[iaxclient](!)`, new users are created in `iax.conf` using the previous format. If the template exists but lacks a valid `#tryinclude` inside its stanza, has multiple destinations, or uses wildcards, an error is reported instead of saving elsewhere. Every successful creation, modification, or deletion sends `iax2 reload` through AMI, including creation of the included file.

Reloads use Amerimon's configured AMI credentials and require permission to execute CLI commands; no `asterisk` binary is executed through browser privileges, and Asterisk is not restarted. **If the file is saved but the reload fails, there is no automatic rollback:** the interface reports the partial save. Check AMI and Asterisk logs, review the content, and reload before repeating the operation. A reload response does not guarantee phone registration or audio; verify the result in Asterisk and on the client.

These pages follow the layout of the other administrative options: a centered container, bordered forms, compact buttons, a trash icon, and a mobile-responsive layout. Passwords use the same shared field with an eye button to show or hide them without saving or changing their value. They are initially masked but are delivered to an authorized administrator's browser to edit the configuration; responses are not cached. Use HTTPS and a trusted device.

The copy icon copies the name or ID from the list without opening the editor, and the current value of non-secret form fields without saving. It displays clipboard confirmation or an error. Passwords and secrets do not include a copy icon.

#### Preserving customized configurations and limitations

IAX does not list `[general]`, `radio`, `allstar-sys`, `allstar-public`, or templates marked with `(!)`. Reserved sections are also protected by the backend. PJSIP lists endpoints and follows their `auth` and `aors` references, even when the objects have different names; global and transport objects are not treated as extensions. Editing preserves unexposed settings, such as transport, authentication username, and custom parameters; exposed settings are updated with the form values, except for IAX users inheriting from `iaxclient`, for which editing changes only the secret.

Only unambiguous sections in the main file can be edited or deleted, with the IAX `#tryinclude` file exception described above. PJSIP requires one `userpass` auth object and one AOR, both in that file and exclusive to the endpoint. Shared, duplicate, or template-inherited objects, multiple or missing references, and sections containing `#include` directives are rejected without writing. Other included files are not traversed: installations split across several files require manual review, including before creating an identifier that might already exist in another include.

New PJSIP extensions create endpoint, auth, and AOR objects with the same ID; new IAX users use form values without a template or inherit from `iaxclient` in included-file mode. Dialplan contexts, transports, and network rules are not created: the selected context and telephony infrastructure must already exist and be compatible with the installation.

### Resetting the administrator account

If the password is forgotten, run this as a user with sudo access:

```bash
sudo amerimon-reset-password
```

The command deletes the administrator record and saved SSH credentials; it does not delete the database, nodes, favorites, connection history, or Asterisk logs. Then open the web app, click **Admin**, and define the username and password again.

Password reset does not rotate `/etc/amerimon/jwt.secret` or revoke JWTs that have already been issued. If all tokens must also be invalidated, back up and regenerate the JWT secret, then restart the service:

```bash
sudo cp -a /etc/amerimon/jwt.secret /etc/amerimon/jwt.secret.bak
sudo sh -c 'umask 077; head -c 48 /dev/urandom | base64 -w0 > /etc/amerimon/jwt.secret'
sudo systemctl restart amerimon
```

The login dialog lets you select **Remember username, password, and session on this device**. If unchecked, the JWT stays in memory and is lost on reload; if checked, it is stored in Local Storage until you sign out or the server rejects it. The checkbox, last username, and password are stored separately in Local Storage even after signing out and are restored when the form is opened. A successful login with the checkbox unchecked removes the preference and remembered credentials. A successful password change updates the remembered password. A failed login does not replace saved credentials. **The password is stored unencrypted and is accessible to page scripts and anyone with access to the browser profile. Use this option only on trusted devices.**

The interface uses only the dark theme, including forms and manual diagrams. Titles, labels, and text must maintain a minimum contrast ratio of 4.5:1 (3:1 for large text); global dark-theme rules retain priority over Angular's encapsulated styles. Printing preserves the paper layout.

If there are multiple servers, **Show Server** lets you select a local node or **All**. On wide screens, the compact selector appears next to **Search Connections**; on mobile, they stack without overflowing.

The compact footer shows the Amerimon Git version and **Powered by Angular Framework, SQLite3, and Go Language** credits, with local logos adapted to the dark theme. `make run` and `make dev-backend` inject `git describe --tags --always`; packages receive the tag version through GoReleaser. A direct `go run` without an injected version displays `dev`. The footer has no legal links or visit counter and is hidden when printing.

### External links

The general **External Links** button is available to all visitors and shows only active records. From **Manage Links**, an administrator can create, edit, and delete `enlaces_externos` records with a deletion confirmation. The `dashid`, `descripcion`, `url`, `target`, and `is_active` fields are preserved; the schema is adapted to SQLite through a new migration without changing existing data. Description and URL accept up to 255 characters; only HTTP/HTTPS URLs and `_blank` (new tab) or `_self` (same tab) targets are allowed.

The Listen Live sessions report no longer shows the **User** and **Callsign** columns. ID, source IP, start time, and authorized actions remain.

| Path | Access | Function |
| --- | --- | --- |
| `/mon/health` | Public | HTTP/DB health |
| `/mon/auth/login`, `/mon/auth/status`, `/mon/auth/setup` | Public | Login and initial administrator setup |
| `/mon/allstarlink/nodes`, `/mon/allstarlink/activity`, `/mon/allstarlink/activity/live` | Public, optional token | Nodes and activity (REST and real-time WebSocket) |
| `/mon/listen-live/*` | Public, token optional depending on operation | Sessions, status, and audio WebSocket |
| `/mon/allmon/config` | Administrator | Read/save Allmon nodes |
| `/mon/iax-users` | Administrator | GET: users keyed by name; POST: create or update an IAX user |
| `/mon/iax-users/:name` | Administrator | DELETE: remove an IAX section only when its name matches an amateur radio callsign |
| `/mon/pjsip-users` | Administrator | GET: endpoints with auth/AOR; POST: create or update an extension |
| `/mon/pjsip-users/:id` | Administrator | DELETE: remove the endpoint and its exclusive auth/AOR objects |
| `/mon/asterisk/commands` | Administrator | List and run Asterisk actions |
| `/mon/asterisk/logs` | Administrator | Latest 500 operations by default, maximum 2000 |
| `/mon/audio/tracks` | Administrator | GET: library; POST: upload audio and convert to WAV |
| `/mon/audio/tracks/:id` | Administrator | PATCH: edit mode/activation; DELETE: remove audio |
| `/mon/audio/tracks/:id/play` | Administrator | Start one-time broadcast |
| `/mon/audio/tracks/:id/file` | Administrator | GET: authenticated WAV for local listening, without broadcasting or changing scheduling |
| `/mon/allstarlink/connections/history` | Administrator | Connection history |
| `/mon/system-info` | Administrator | System, CPU, memory, disks, network, temperature, voltage, and Asterisk |
| `/mon/site-settings` | Public GET; administrator PUT | Header title/subtitle |
| `/mon/favoritos` | Administrator | Favorites |
| `/mon/enlaces-externos` | Public | Active links only |
| `/mon/admin/enlaces-externos`, `/mon/admin/enlaces-externos/:id` | Administrator | External links CRUD |

## 9. Asterisk operation logs

Operations from the AllStarLink panel are stored in `asterisk_operation_logs` and appear under **Administration → Asterisk Logs**. The stored fields are UTC date, action, local/remote node, mode, command sent, result, details, and duration. Details are limited to 2000 characters.

Results distinguish, among other things, invalid requests, configuration errors, AMI errors, accepted commands with unconfirmed links, and success. Process logs also use the `asterisk:` prefix and can be queried with:

```bash
sudo journalctl -u amerimon -f | grep 'asterisk:'
```

`AMI_SECRET`, passwords, and tokens are not stored. The endpoint and log page are restricted to administrators. The list provides up to 2000 events; the application does not automatically purge them, so the database must be included in the backup/retention policy.

This history covers command/connection panel actions and Listen Live start/stop attempts (`listen_start`, `listen_stop`), including AMI errors. A successful start means AMI accepted `Originate`, not that audio is already arriving. It does not replace Asterisk logs.

Audio management records one-time and scheduled broadcasts and AMI failures. The delay between playbacks is calculated from validated WAV duration and persisted in `audio_playback_state`.

### Diagnosing Listen Live on Raspberry Pi (Debian 12 and 13)

After installing the package, open **Asterisk Logs** to review listening attempts. To follow the full flow, run this over SSH before clicking **LISTEN LIVE**:

```bash
sudo journalctl -u amerimon -f -o short-iso
```

The logs record the start request, AMI acceptance or rejection, WebSocket connection, EAGI connection, first audio block, and total blocks/bytes at the end. The UUID correlates stages without logging credentials or audio content. If `EOF` appears, the message identifies whether the connection closed during the banner, login, or `Originate`; a rejected login is shown as such, and no later originate is attempted.

To collect evidence from a failed test and copy it from the Raspberry Pi:

```bash
sudo journalctl -u amerimon --since "15 minutes ago" -o short-iso --no-pager > /tmp/amerimon-diagnostico.log
sudo journalctl -u asterisk --since "15 minutes ago" -o short-iso --no-pager > /tmp/asterisk-diagnostico.log
uname -m
dpkg-query -W amerimon asterisk
sudo ss -ltnp '( sport = :8082 or sport = :8086 )'
```

If Asterisk writes to a file instead of the journal, also check `/var/log/asterisk/messages.log`. Startup messages show the installed version. Do not share `amerimon.env`, `jwt.secret`, or `manager.conf`; they contain credentials.

## 10. System information

### Administrative maintenance

Under **Admin → System**, the administrator can **Restart Asterisk**, **Reboot Linux**, or **Power Off Linux**. Each action requires entering its confirmation phrase; all interrupt communications. The `POST /mon/system/control` API requires an administrator-role JWT and accepts only these three fixed actions, never arbitrary commands.

- **Asterisk:** attempts `systemctl restart asterisk.service`. If that fails, it requests the service to stop and sends SIGTERM only to processes in that unit; if they do not exit, it uses SIGKILL. It does not search by process name or terminate unrelated processes. It runs `start` only after confirming the stop; then it verifies the service is active and AMI is responsive through uptime. A recovery or verification failure is returned as an error, not reported as a successful restart. The total deadline is approximately one minute.
- **Diagnostics:** displays `asterisk.service` journal errors since the request and recent errors from the standard `/var/log/asterisk/messages.log` file (last 256 KiB, up to 40 lines). If either source cannot be queried, this is indicated. Startup errors are shown as warnings even when the service and AMI respond; for example, a USB interface may be missing. A custom log file must be checked separately.
- **Linux:** schedules `shutdown -r +1` or `shutdown -P +1`, with a one-minute delay. The response confirms that the action was scheduled, not that the reboot/power-off completed. Powering off may require physical access to turn the device back on. To cancel from a console before it occurs, run `sudo shutdown -c`.

Requests and results are recorded in **Asterisk Logs** and Amerimon's journal. The installed service runs as root; a development instance without those permissions returns a system error and does not attempt privilege escalation. Do not test Linux actions on shared or production equipment without authorization. Tests for these actions simulate every command; they do not reboot or power off the machine.

The **System** popup requires an administrator session. It includes:

- Host, distribution, architecture, kernel, and uptime.
- CPU, load, memory, storage, and swap.
- Network interfaces and traffic.
- Processes with the highest CPU usage.
- Temperature read from `/sys/class/thermal/thermal_zone*`.
- Voltage: `vcgencmd measure_volts core` on ARM; `hwmon` reading on AMD.
- Asterisk version obtained through AMI with `core show version`.

Temperature, voltage, or version may be unavailable if the kernel, hardware, or AMI does not expose the data.

## 11. Apache, Nginx, and web access

The post-install script attempts to detect the active web server. If it configures the proxy correctly, use `http(s)://<host>/amerimon/`. It does not install TLS certificates: HTTPS works only if the host already has SSL configured.

### Apache

The installation enables `proxy`, `proxy_http`, `proxy_wstunnel`, `headers`, and `alias`, enables `conf-available/amerimon.conf`, runs `apache2ctl configtest`, and reloads/starts Apache only if the test passes. The proxy handles HTTP/HTTPS according to the VirtualHost receiving the request, including prefix headers and WebSocket.

Checks:

```bash
sudo apache2ctl configtest
sudo apache2ctl -S
ls -l /etc/apache2/conf-enabled/amerimon.conf
curl -i http://127.0.0.1/amerimon/mon/health
```

An `AH00558` warning about `ServerName` is not a failure if `configtest` ends with `Syntax OK`.

### Nginx

The package includes `/usr/lib/amerimon/configure-nginx.sh` and a snippet at `/usr/share/amerimon/nginx-amerimon.conf.example`. The helper looks for the default HTTP VirtualHost (or the first one with `listen 80`), inserts the `include`, runs `nginx -t`, and reverts the file if the test or reload fails.

The snippet defines `/amerimon/`, the WebSocket, and proxy headers. Nginx must have a valid HTTP `server` block. If the site uses a separate TLS block on `443`, verify that `include /etc/nginx/snippets/amerimon.conf;` is also inside that block so HTTPS works. The package does not create or renew certificates.

Checks:

```bash
sudo nginx -t
sudo nginx -T | grep -n -A8 -B4 amerimon
curl -i http://127.0.0.1/amerimon/mon/health
```

If neither Apache nor Nginx is present, the package should not fail for that reason: the application remains available on `:8082`, without the `/amerimon` path or TLS.

## 12. AstDB updates

`amerimon-astdb.timer` is enabled during installation and runs `astdb.sh` every five minutes, with a randomized startup delay. The script downloads `https://allmondb.allstarlink.org/allmondb.php`, merges `privatenodes.txt` if it exists, and writes `astdb.txt` to the Supermon directory if present; otherwise, it uses `/var/lib/asterisk/`.

```bash
systemctl status amerimon-astdb.timer
systemctl list-timers amerimon-astdb.timer
sudo systemctl start amerimon-astdb.service
sudo journalctl -u amerimon-astdb.service -n 50 --no-pager
```

The YAML must point `sources.astdb_path` to the file that is actually updated.

## 13. Local development (developers only)

Before starting development mode, stop the installed service to free ports `8082` and `8086`:

```bash
sudo systemctl stop amerimon
make run
```

`make run` compiles Angular, embeds it, and runs Go at `127.0.0.1:8082`, with database `/tmp/amerimon-dev.db` and JWT secret `dev-secret`. It uses credentials from `/etc/asterisk/manager.conf` and `packaging/amerimon.yaml`. Stop it with `Ctrl+C`.

To switch to the installed service using the current code:

```bash
./install.sh
```

`install.sh` recompiles the frontend/backend, updates `/usr/bin/amerimon`, installs the EAGI script, dialplan migration, and service support files. It runs the same `packaging/postinstall.sh` as the Debian package to migrate Listen Live with a backup, enable available modules, reload the dialplan, and restart Amerimon even if it is already active. It preserves existing configuration, credentials, and Allmon files. It also applies the package's web proxy configuration and AstDB timer. Do not run `make run` and `amerimon.service` simultaneously because they share ports.

For separate frontend and backend reloading, use two terminals: `make dev-backend` and `npm start`.

## 14. Build and release (developers only)

This section is for the team that develops and publishes Amerimon only. **It does not apply to clients**: install or update with APT as described in section 4. `make dist` requires a clean Git tree and performs an actual release. It:

1. Calculates and pushes the next semver tag.
2. Runs `goreleaser release --clean`; its hooks run `go mod tidy`, `npm ci`, and `npm run build`.
3. Builds `.deb` packages for Linux `amd64` and `arm64` with CGO disabled.
4. Copies packages to the local repository, keeps up to three versions per architecture, regenerates/signs indexes, and pushes to `jjsainzc/repo-deb`.

Related commands:

```bash
make dist
make dist VERSION=v1.2.0
make index-repo
make push-repo
```

Before providing APT instructions, confirm that `https://jjsainzc.github.io/repo-deb/Packages` includes the published version. On client machines, verify with `apt-cache policy amerimon` that `Candidate` matches.

## 15. Quick troubleshooting

| Symptom | Checks |
| --- | --- |
| Service does not start | `systemctl status amerimon`; `journalctl -u amerimon -n 100`; check `8082` and `8086` |
| Local node does not appear | Confirm `rpt_config_path`, numeric sections, and the primary node's loopback entry in `rpt.conf`; Allmon provides remote servers only |
| AMI rejects an operation | Check `AMI_USER`/`AMI_SECRET` without printing them; validate host/port and permissions for the existing user; inspect `journalctl -u amerimon` |
| IAX/PJSIP cannot load or save | Check configuration paths, file/directory permissions, and the administrator session; use **Reload list** and read the diagnostic |
| IAX/PJSIP returns a conflict while editing or deleting | Check duplicates, templates, includes, missing, multiple, or shared auth/AOR references; resolve manually without removing customer settings |
| IAX/PJSIP reports a saved file but a failed reload | The file has already changed; check AMI and Asterisk logs, review the content, and reload before retrying; there is no automatic rollback |
| AMI accepts but does not confirm a connection | Open Asterisk Logs and review `XStat` details, remote status, and timeout |
| Notice about the same mode and permanence | The link already has the requested settings; no commands were sent. Change mode or permanence only if reconnection is actually intended |
| New connection was not started when changing settings | Review Asterisk Logs for the old link's disconnection, confirmation of its absence in `XStat`, and possible alternate routes; a removed link is not automatically restored |
| `/amerimon` returns 404 | Confirm Apache/Nginx configuration is enabled, the path is correct, `configtest` passes, and the correct VirtualHost proxy is used |
| `apt` offers an older version | Run `apt update`, check `apt-cache policy amerimon`, and confirm the public Pages index |
| Temperature/voltage unavailable | Check kernel sensors, `vcgencmd` on ARM, or `hwmon` entries on AMD |

Do not edit Asterisk to resolve a web-proxy error; also do not expose AMI port `5038` outside the host without a specific security design.
