# Amerimon Technical Operations Manual

## 1. Scope

Amerimon is a web application for querying AllStarLink, managing Allmon nodes, listening to live audio, and performing authorized Asterisk operations. The Angular frontend is compiled and embedded in a single Go service.

This manual describes the current source-code tree. The reference published Debian version is `1.0.4`. Clients install and update Amerimon with APT; they do not need to compile the code or run publishing tasks.

Amerimon queries Asterisk through AMI. Its post-install script prepares the modules and contexts required for Listen Live. An administrator can explicitly change the primary node's identity and access lists, and manage IAX users and PJSIP extensions from their dedicated pages. Other nodes and telephony users are not reconfigured automatically.

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
| `asterisk_operation_logs` | Asterisk operations, results, and diagnostics |

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
| `/lib/systemd/system/amerimon.service` | systemd unit |
| `/usr/local/bin/astdb.sh` | AstDB node database updater |

`amerimon.env` and `amerimon.yaml` are installed as `config|noreplace`; an upgrade preserves existing local files.

By default, `sources.privatenodes_path` uses `/var/lib/asterisk/privatenodes.txt`. At startup, Amerimon preserves any existing file at the configured path; if it is elsewhere and the default does not exist, Amerimon creates a symbolic link to it. If the default path is selected and the file is missing or empty, Amerimon first searches for files containing data at `/etc/allmon3/privatenodes.txt` and `/var/www/html/supermon/privatenodes.txt`, linking them so existing nodes can be displayed and edited without duplication. It never replaces a file containing data or an existing link. If no previous file exists, it creates the default file. The administrator can edit, add, and delete lines from **Private Nodes**; saving preserves the shared file's permissions and group. In production, the service runs as root and also preserves the owner; in unprivileged development, the replacement may become owned by the process user, and this is recorded in the log. Each entry uses `node|callsign|description|location`.

Upgrades do not replace `/etc/amerimon/allmon.ini`: the installer creates it only if it is missing. The Allmon node editor preserves sections it does not manage and creates an `.amerimon.bak` copy before saving.

### Primary node configuration

**Admin → Primary Node** provides functionality equivalent to options **1 Node number**, **2 Node Password**, **3 Node callsign**, and **7 Node access list** in `node-setup`.

- **Number:** changes the public number of the primary node identified in `rpt.conf`. This monitor requires a public primary node (4 to 6 digits, not starting with 0 or 1); private nodes are managed as connections, not as the primary. Numbers already configured and forms left open for a primary node that has changed are rejected.
- **Password:** this is the registration password obtained from the AllStarLink portal, not the Amerimon or AMI password. Only an administrator can retrieve the current password; it is hidden and can be revealed with the eye button. The active HTTP registration is preferred, falling back to the IAX registration if needed. The response is not cached; the password is not stored in browser storage or logs. Use HTTPS and a trusted device. Leaving the field unchanged or empty preserves the current password; a new password may contain 6 to 128 letters, numbers, hyphens, and underscores.
- **Callsign:** updates `idrecording = |iCALLSIGN` directly in the primary section while preserving templates. An inherited `idrecording` or one based on an audio file is not presented as an editable callsign; enter the callsign you want to use.
- **Access:** adds, updates, and deletes numbers or callsigns in AstDB's `allowlist/<primary>` and `denylist/<primary>`, with an optional reason. Changes are applied immediately through AMI. With no entries, these lists impose no restrictions. An allowlist limits which nodes are accepted; blocked nodes are rejected. Reasons can contain up to 200 characters and cannot contain double quotes, backslashes, or control characters.
- **AMI access:** before the access lists, you can change the `secret` of the AMI user already configured in `ami.user`, the `[general]` `bindaddr`, and optionally add `permit`/`deny` networks to that same stanza. The secret is masked and can be revealed with the eye button. IPv4 networks accept an IP/prefix or IP/netmask; Amerimon calculates the normalized network. Existing rules are preserved only: the screen adds rules but does not delete or replace custom options. Any `deny` that covers loopback or the backend address is rejected, and explicit loopback access is added when restricting access with `permit`. If the backend AMI address cannot be resolved, ACL changes are not applied.
- This editor creates `.amerimon.bak` backups of `manager.conf`, `amerimon.yaml`, and `amerimon.env`; updates `ami.host`, `ami.user`, and the secure `ami.secret: ${AMI_SECRET}` reference in YAML; and keeps the secret value in the environment file with its existing permissions. The installed service explicitly specifies `/etc/amerimon/amerimon.env`; for custom installations, set `AMERIMON_ENV_FILE` to the environment file used by the process. When a specific `bindaddr` is used, the YAML AMI host is synchronized with that address; with `0.0.0.0` or `::`, the current host is retained. Amerimon then runs `manager reload` (without restarting Asterisk) and checks a new AMI connection. If authentication fails, it attempts to restore the files and reload the previous configuration. Do not restart Asterisk if a restoration failure is reported.

Editing changes only references to the selected node in `rpt.conf`, `rpt_http_registrations.conf`, `iax.conf`, and, if present, `savenode.conf`, `extensions.conf`, `simpleusb.conf`, `usbradio.conf`, and `voter.conf`. It also renames the corresponding section in the active Allmon file and in `/etc/allmon3/allmon3.ini`, when present. Other sections, registrations, and credentials are preserved. When renumbering, access lists are moved to the new number; a destination that already has lists is rejected.

Edited definitions must be directly in those files: `#include` files, arbitrary customizations, and numeric references in external scripts are not rewritten. If the primary section is not defined directly, saving is rejected. The process must be able to read the files, create backups, and replace them in their directories while preserving owner, group, and permissions. The web application does not elevate privileges. Asterisk paths are derived from the directory of `ami.rpt_config_path`.

Before modifying files, adjacent `*.amerimon.bak` backups are created with mode `0600`; they contain passwords and must not be published or copied to locations accessible over HTTP. The latest backup of each file is kept. If saving or migrating lists fails, Amerimon attempts to restore the previous configuration and explicitly reports any restoration failure. If a restoration failure is reported, do not restart Asterisk; check the backups first.

**Saving does not restart or reload Asterisk.** After verifying the data, restart it manually from **System**. Restarting interrupts active links and listening sessions. If you changed the number and use Allmon3, restart Allmon3 manually as well. Until the restart, Asterisk still uses its previous identity; apply changes during a maintenance window.

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

If `/etc/asterisk` exists, the post-install script saves one copy of the previous `modules.conf` (`modules.conf.bak-listen-live`) and enables the modules present among `res_speech.so`, `res_agi.so`, `app_chanspy.so`, `app_mixmonitor.so`, `app_verbose.so`, `func_config.so`, `func_strings.so`, `func_logic.so`, and `func_cut.so`. `app_verbose.so` provides the context's `Verbose` and `Log` applications; if it is not loaded, Asterisk ends the call before reaching ChanSpy. The script converts a `noload` directive for these modules to `load`, or appends `load => ...`. If `extensions.conf` exists, it saves `extensions.conf.bak-listen-live` and adds the `[listen-live-spy]`, `[listen-live-eagi]`, and `[amerimon-listen-node]` contexts if missing. It migrates only recognized legacy Amerimon `nodo` blocks (SimpleUSB or Local/pseudo), replacing them with a single redirect to the new context; custom blocks are preserved and generate a warning. Before migration, it also saves `extensions.conf.bak-listen-live-rxchannel`.

If Asterisk is already running, the hook attempts to load these modules and run `dialplan reload`; it does not restart the Asterisk service. If a module does not exist in that distribution, a warning is logged and the Amerimon installation continues. The `.bak-listen-live` copies can be used to review or manually restore the previous state.

### Allmon node selection and syntax

`allmon.ini` is searched in this order:

1. `sources.allmon_ini_path`, if explicitly configured.
2. `/etc/allmon3/allmon.ini`.
3. `/var/www/html/supermon/allmon.ini`.
4. `sources.allmon_ini_own_path`; defaults to `/etc/amerimon/allmon.ini`.

Local nodes are obtained from `/etc/asterisk/rpt.conf` (or `ami.rpt_config_path`), including the primary identified by `<node> = radio@127.0.0.1/<node>,NONE` in `[nodes]`. Nodes in `privatenodes.txt` and those classified as private (four digits) are excluded as servers: they do not create tables or server-selector options. Their connections still appear in the table for the node to which they are connected, whether the primary or another server. The primary is not selected by order: if several non-private entries match the loopback rule, an ambiguity error is logged. Local nodes use Amerimon's global AMI credentials. Allmon entries pointing to loopback or the global AMI host do not replace these nodes or their credentials. Remote server instances retain their Allmon configuration. This selection is shared by the table, activity polling, and node operations.

In the AllStarLink monitor, the primary node's table appears first; the rest are sorted by number. Each table header shows `number - description - IP/hostname`; the primary keeps its **Primary node** label, and remote tables do not have a **Remote node** prefix. The description comes from `astdb.txt`, and the address comes from the global AMI host for local nodes or Allmon's `host` field for remote nodes (without the port). The configured IP or hostname is displayed without DNS lookups; missing data is omitted, even when the server is disconnected.

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

### IAX users and PJSIP extensions

Open **Admin → More options → IAX users / PJSIP extensions**, at `/admin/iax-users` and `/admin/pjsip-users`. These are administrative pages, not modal dialogs, and load their lists when opened. Titles, labels, buttons, empty states, loading messages, generic errors, and deletion confirmations follow the Spanish or English language selected in the header, including when switching languages with a form open. Configuration values are not translated. Diagnostics returned by the backend are preserved verbatim so technical information is not lost.

1. Click **New IAX user** or **New PJSIP extension**, or select a list entry to edit it.
2. IAX exposes **Name (section)**, **Context**, **Secret (password)**, and **Authentication**. PJSIP exposes **Extension number (ID)**, **Context**, **Password**, **Caller ID**, and **Maximum contacts**.
3. The name or ID is locked while editing; renaming is not supported by these forms. Context and secret or password are required; maximum contacts must be a positive integer. Identifiers accept 1 to 80 ASCII letters, numbers, periods, hyphens, and underscores; IAX reserves `general`. The backend rejects newlines, null characters, and the `;` and `#` comment delimiters in submitted values.
4. **Save** applies changes and reloads the list. If the IAX name or PJSIP ID does not match the recognized amateur radio callsign format, a confirmation is requested because the section may belong to system configuration. Recognized callsigns are saved directly. **Cancel** closes the editor without saving. **Delete** is available while editing and requests confirmation containing the entry's identifier.
5. The save confirmation for sections that do not look like callsigns is shared by IAX and PJSIP. You can select **Do not show this type of notice again**; the preference is stored in this browser and future saves of this type proceed without asking. It can also be reset from **Admin → Customize Header → Reset warnings and confirmations**.
6. If an operation fails, the error is displayed and the edited data is retained for correction or retry. **Reload list** retries the query without saving the form. Saving and deleting are disabled while another modification is in progress.

#### Files and reloads

| Module | File | AMI reload |
| --- | --- | --- |
| IAX | `iax.conf` in the directory containing `ami.rpt_config_path`; defaults to `/etc/asterisk/iax.conf` | `iax2 reload` |
| PJSIP | `ami.pjsip_config_path`; defaults to `/etc/asterisk/pjsip.conf`. If empty, it is located alongside `ami.rpt_config_path` | `pjsip reload` |

The service user needs to read and write the files and create files in their directory. Before saving, an `<file>.amerimon.bak` backup is created or replaced with mode `0600`; configuration replacement is atomic, resolves symbolic links, and preserves owner, group, and permissions. If backup creation fails, the configuration is not changed. Include these files and backups, which contain passwords, in a restricted-access and backup policy; they are independent of SQLite.

Reloads use Amerimon's configured AMI credentials and require permission to execute CLI commands; no `asterisk` binary is executed through browser privileges, and Asterisk is not restarted. **If the file is saved but the reload fails, there is no automatic rollback:** the interface reports the partial save. Check AMI and Asterisk logs, review the content, and reload before repeating the operation. A reload response does not guarantee phone registration or audio; verify the result in Asterisk and on the client.

These pages follow the layout of the other administrative options: a centered container, bordered forms, compact buttons, a trash icon, and a mobile-responsive layout. Passwords use the same shared field with an eye button to show or hide them without saving or changing their value. They are initially masked but are delivered to an authorized administrator's browser to edit the configuration; responses are not cached. Use HTTPS and a trusted device.

The copy icon copies the name or ID from the list without opening the editor, and the current value of non-secret form fields without saving. It displays clipboard confirmation or an error. Passwords and secrets do not include a copy icon.

#### Preserving customized configurations and limitations

IAX does not list `[general]` or templates marked with `(!)`. PJSIP lists endpoints and follows their `auth` and `aors` references, even when the objects have different names; global and transport objects are not treated as extensions. Editing preserves unexposed settings, such as transport, authentication username, and custom parameters; exposed settings are updated with the form values.

Only unambiguous sections in the main file can be edited or deleted. PJSIP requires one `userpass` auth object and one AOR, both in that file and exclusive to the endpoint. Shared, duplicate, or template-inherited objects, multiple or missing references, and sections containing `#include` directives are rejected without writing. Included files are not traversed or edited: installations split across several files require manual review, including before creating an identifier that might already exist in an include.

New PJSIP extensions create endpoint, auth, and AOR objects with the same ID; new IAX users use the form defaults. Dialplan contexts, transports, and network rules are not created: the selected context and telephony infrastructure must already exist and be compatible with the installation.

### Resetting the administrator account

If the password is forgotten, run this as a user with sudo access:

```bash
sudo amerimon-reset-password
```

The command deletes the administrator record from the `admin` table; it does not delete the database, nodes, favorites, connection history, or Asterisk logs. Then open the web app, click **Admin**, and define the username and password again.

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
| `/mon/allstarlink/nodes`, `/mon/allstarlink/activity` | Public, optional token | Nodes and activity |
| `/mon/listen-live/*` | Public, token optional depending on operation | Sessions, status, and audio WebSocket |
| `/mon/allmon/config` | Administrator | Read/save Allmon nodes |
| `/mon/iax-users` | Administrator | GET: users keyed by name; POST: create or update an IAX user |
| `/mon/iax-users/:name` | Administrator | DELETE: remove an IAX section only when its name matches an amateur radio callsign |
| `/mon/pjsip-users` | Administrator | GET: endpoints with auth/AOR; POST: create or update an extension |
| `/mon/pjsip-users/:id` | Administrator | DELETE: remove the endpoint and its exclusive auth/AOR objects |
| `/mon/asterisk/commands` | Administrator | List and run Asterisk actions |
| `/mon/asterisk/logs` | Administrator | Latest 500 operations by default, maximum 2000 |
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
