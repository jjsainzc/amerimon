# Manual de Operación Técnica de Amerimon

## 1. Alcance

Amerimon es una aplicación web para consultar AllStarLink, administrar nodos Allmon, escuchar audio en vivo y ejecutar operaciones autorizadas en Asterisk. El frontend Angular se compila y se incrusta en un único servicio Go.

Este manual describe el árbol de código actual. La versión Debian publicada de referencia es `1.0.4`. Los clientes instalan y actualizan Amerimon con APT; no necesitan compilar el código ni ejecutar tareas de publicación.

Amerimon consulta Asterisk mediante AMI. Su postinstalador prepara los módulos y contextos necesarios para Listen Live. El administrador puede cambiar explícitamente la identidad y las listas de acceso del nodo principal desde el panel web; no se reconfiguran automáticamente los demás nodos ni PJSIP.

## 2. Componentes y puertos

| Componente | Dirección predeterminada | Uso |
| --- | --- | --- |
| HTTP de Amerimon | `0.0.0.0:8082` en el paquete; `127.0.0.1:8082` en desarrollo | Frontend embebido y API REST `/mon` |
| Audio EAGI | `127.0.0.1:8086` | Entrada local de audio para Listen Live |
| Asterisk AMI | `127.0.0.1:5038` | Consultas y operaciones de Asterisk |
| Angular dev server | `localhost:4200` | Solo desarrollo con `npm start` |

El puerto `8086` debe estar libre al iniciar Amerimon. El proceso EAGI y la conexión AMI son locales al servidor. Si no se instala Apache o Nginx, Amerimon sigue funcionando directamente en `http://<IP-del-servidor>:8082`; sin proxy no existe la ruta `/amerimon` ni HTTPS.

## 3. Tecnologías y datos

- Backend: Go `1.26`, Echo y WebSocket.
- Frontend: Angular 22, TypeScript 6 y Vitest.
- Base de datos: SQLite mediante `modernc.org/sqlite`, implementación Go pura, sin servidor SQLite independiente ni CGO.
- La base usa WAL, `busy_timeout` y claves foráneas.
- Los esquemas se aplican mediante migraciones SQL embebidas. No se deben editar migraciones ya publicadas; se agrega una nueva migración numerada.
- Antes de migrar una base existente, Amerimon crea una copia `*.bak-<fecha>` y conserva las tres copias más recientes.

### Tablas principales

| Tabla | Propósito |
| --- | --- |
| `admin` | Usuario administrador y hash de contraseña |
| `users`, `listenlive` | Sesiones y usuarios de Listen Live |
| `asterisk_commands` | Catálogo de operaciones permitidas |
| `allstarlink_node_activity` | Última actividad observada entre nodos |
| `asterisk_connection_history` | Historial de enlaces observados |
| `favoritos` | Nodos favoritos de administración |
| `enlaces_externos` | Enlaces publicados y administrados desde la web |
| `app_settings` | Título y subtítulo de cabecera |
| `asterisk_operation_logs` | Operaciones Asterisk, resultado y diagnóstico |

## 4. Instalación Debian

El repositorio APT público es `https://jjsainzc.github.io/repo-deb/` y está firmado con la clave `hc1lnm.gpg`. En una Raspberry Pi ARM64 o Debian AMD64:

```bash
curl -fsSL https://raw.githubusercontent.com/jjsainzc/repo-deb/main/hc1lnm.gpg \
  | sudo gpg --dearmor -o /usr/share/keyrings/hc1lnm-archive-keyring.gpg

echo "deb [signed-by=/usr/share/keyrings/hc1lnm-archive-keyring.gpg arch=$(dpkg --print-architecture)] https://jjsainzc.github.io/repo-deb/ ./" \
  | sudo tee /etc/apt/sources.list.d/amerinet.list

sudo apt update
apt-cache policy amerimon
sudo apt install amerimon
```

Para una versión nueva publicada:

```bash
sudo apt update
apt list --upgradable
sudo apt upgrade
```

Comprueba la versión y el servicio:

```bash
dpkg-query -W -f='${Version}\n' amerimon
sudo systemctl status amerimon --no-pager
```

Después de que el mantenedor publique una versión, el índice APT puede tardar en actualizarse. En la Raspberry, confirma con `apt-cache policy amerimon` que `Candidate` muestre la versión esperada antes de actualizar.

### Archivos instalados

| Ruta | Contenido |
| --- | --- |
| `/usr/bin/amerimon` | Backend Go con frontend embebido |
| `/etc/amerimon/amerimon.env` | Dirección, base de datos y credenciales AMI |
| `/etc/amerimon/amerimon.yaml` | AMI, rutas y fuentes de datos |
| `/etc/amerimon/jwt.secret` | Clave JWT creada aleatoriamente, permisos `0600` |
| `/etc/amerimon/allmon.ini` | Configuración propia de nodos si no existe otra activa |
| `/var/lib/asterisk/privatenodes.txt` | Nodos privados administrados desde el panel web |
| `/var/lib/amerimon/amerimon.db` | Base SQLite de producción |
| `/lib/systemd/system/amerimon.service` | Unidad systemd |
| `/usr/local/bin/astdb.sh` | Actualización de la base de nodos AstDB |

Las configuraciones `amerimon.env` y `amerimon.yaml` se instalan como `config|noreplace`; una actualización conserva los archivos locales existentes.

`sources.privatenodes_path` usa `/var/lib/asterisk/privatenodes.txt` de forma predeterminada. Al iniciar, Amerimon conserva el archivo ya existente en la ruta configurada; si está en otra ubicación y el predeterminado aún no existe, crea un enlace simbólico hacia él. Si se usa la ruta predeterminada y falta o está vacía, busca primero archivos con contenido en `/etc/allmon3/privatenodes.txt` y `/var/www/html/supermon/privatenodes.txt`, enlazándolos para mostrar y editar los nodos existentes sin duplicarlos. Nunca reemplaza un archivo con datos ni un enlace existente. Si no hay ningún archivo previo, crea el predeterminado. El administrador puede editar, agregar y borrar líneas desde **Nodos privados**; al guardar se conservan los permisos y el grupo del archivo compartido. El servicio de producción, ejecutado como root, conserva además su propietario; en desarrollo sin privilegios, el reemplazo puede pasar al usuario del proceso y se registra esta situación en el log. Cada registro usa `nodo|indicativo|descripción|ubicación`.

Las actualizaciones no reemplazan `/etc/amerimon/allmon.ini`: el instalador lo crea solo si falta. El editor de nodos Allmon conserva las secciones que no administra y genera una copia `.amerimon.bak` antes de guardar.

### Configuración del nodo principal

En **Admin → Nodo principal** están disponibles las funciones equivalentes a las opciones **1 Node number**, **2 Node Password**, **3 Node callsign** y **7 Node access list** de `node-setup`.

- **Número:** cambia el número público del nodo principal identificado en `rpt.conf`. Este monitor exige un nodo principal público (4 a 6 dígitos sin comenzar por 0 o 1); los nodos privados se administran como conexiones, no como principal. Se rechazan números ya configurados y formularios abiertos para un principal que haya cambiado.
- **Contraseña:** es la contraseña de registro obtenida del portal AllStarLink, no la de Amerimon ni la de AMI. Solo el administrador puede recuperar la contraseña actual, que aparece oculta y puede mostrarse con el botón del ojo. Se prioriza el registro HTTP activo y, si falta, el registro IAX. La respuesta no se almacena en caché; la contraseña no se guarda en el almacenamiento del navegador ni en los registros. Utiliza HTTPS y un dispositivo de confianza. Dejar el campo sin cambios o vacío conserva la actual; una nueva contraseña admite entre 6 y 128 letras, números, guion y guion bajo.
- **Indicativo:** actualiza `idrecording = |iINDICATIVO` directamente en la sección principal, conservando las plantillas. Un `idrecording` heredado o basado en un archivo de audio no se presenta como un indicativo editable: introduce el indicativo que quieras utilizar.
- **Acceso:** permite agregar, actualizar y eliminar números o indicativos en `allowlist/<principal>` y `denylist/<principal>` de AstDB, con un motivo opcional. Los cambios se aplican inmediatamente mediante AMI. Sin entradas no hay restricciones de estas listas. Una lista de permitidos limita los nodos admitidos; los bloqueados se rechazan. Los motivos admiten hasta 200 caracteres, sin comillas dobles, barras inversas ni caracteres de control.

La edición modifica únicamente referencias al nodo seleccionado en `rpt.conf`, `rpt_http_registrations.conf`, `iax.conf` y, si existen, `savenode.conf`, `extensions.conf`, `simpleusb.conf`, `usbradio.conf` y `voter.conf`. También renombra la sección correspondiente en el Allmon activo y en `/etc/allmon3/allmon3.ini`, cuando existen. Las demás secciones, registros y credenciales se conservan. Al renumerar, las listas de acceso se trasladan al nuevo número; se rechaza un destino que ya tenga listas.

Las definiciones editadas deben estar directamente en esos archivos: no se reescriben archivos `#include`, personalizaciones arbitrarias ni referencias numéricas en scripts externos. Si falta la sección principal directa, el guardado se rechaza. El proceso debe poder leer los archivos, crear respaldos y reemplazarlos en sus directorios, conservando propietario, grupo y permisos. No se elevan privilegios desde la web. Las rutas de Asterisk se derivan del directorio de `ami.rpt_config_path`.

Antes de modificar archivos se generan respaldos adyacentes `*.amerimon.bak` con permisos `0600`; contienen contraseñas y no deben publicarse ni copiarse a ubicaciones accesibles por HTTP. Se conserva el último respaldo de cada archivo. Si falla el guardado o la migración de listas, se intenta restaurar la configuración anterior y se informa explícitamente si alguna restauración falla. No reinicies Asterisk si se ha notificado un fallo de restauración: revisa los respaldos primero.

**Guardar no reinicia ni recarga Asterisk.** Tras verificar los datos, reinícialo manualmente desde **Sistema**. El reinicio interrumpe los enlaces y las escuchas activas. Si cambiaste el número y usas Allmon3, reinicia también Allmon3 manualmente. Durante el intervalo previo al reinicio, Asterisk aún utiliza su identidad anterior; aplica los cambios en una ventana de mantenimiento.

Los botones **Vaciar permitidos** y **Vaciar denegados** eliminan exclusivamente las entradas de la lista elegida del nodo principal, previa confirmación, sin tocar la otra lista ni otros nodos. El cambio es inmediato y no tiene deshacer. Vaciar permitidos elimina la restricción a los nodos autorizados; la lista de denegados sigue aplicándose. Si falla una eliminación, se informa que el vaciado puede ser parcial y debe recargarse la lista.

**Exportar listas** descarga ambas listas y sus motivos en `listas-acceso-<nodo>.json`, consultando AstDB de nuevo. El documento usa `format: "amerimon-node-access"`, `version: 1`, `node` (origen) y `entries` con `type`, `node` y `reason`. No incluye contraseñas. **Importar listas** permite seleccionar ese JSON (hasta 1 MiB y 1000 entradas) y confirmar el nodo de destino. Siempre combina: conserva entradas ajenas al archivo y actualiza los motivos de las coincidentes por lista y nodo/indicativo. Puede importar un archivo de otro nodo, pero se aplica únicamente al principal actual. No permite entradas duplicadas en una misma lista ni incluir el propio nodo de destino. Valida el archivo completo antes de escribir, verifica las entradas guardadas e intenta restaurar los cambios si falla una operación, notificando cualquier restauración incompleta. Los cambios son inmediatos. La exportación requiere `GET /mon/primary-node/access/export`; la importación, `POST /mon/primary-node/access/import` con `primary_node` y `document`, ambos restringidos al administrador.

La API requiere JWT con rol administrador para `GET/PUT /mon/primary-node`, `GET/PUT/DELETE /mon/primary-node/access` y `POST /mon/primary-node/access/clear`. El vaciado requiere `primary_node`, `type` (`allowlist` o `denylist`) y `confirmation` (`VACIAR allowlist` o `VACIAR denylist`). Las pruebas de esta funcionalidad usan archivos temporales y AMI simulado, sin modificar la configuración del nodo real.

## 5. Servicio systemd

La unidad es `amerimon.service`. Se inicia automáticamente al instalar el paquete y se reinicia si falla.

```bash
sudo systemctl start amerimon
sudo systemctl stop amerimon
sudo systemctl restart amerimon
sudo systemctl status amerimon --no-pager
sudo journalctl -u amerimon -f
```

El servicio recibe `jwt.secret` como credencial de systemd y ejecuta el binario como root, necesario para las rutas del sistema y la integración EAGI configurada.

### Variables de entorno

`/etc/amerimon/amerimon.env` admite:

| Variable | Predeterminado del paquete | Descripción |
| --- | --- | --- |
| `AMERIMON_ADDR` | `0.0.0.0:8082` | Interfaz y puerto HTTP |
| `AMERIMON_DB` | `/var/lib/amerimon/amerimon.db` | Archivo SQLite |
| `AMI_USER` | Vacío hasta detectarlo | Usuario AMI |
| `AMI_SECRET` | Vacío hasta detectarlo | Secreto AMI |
| `AMERIMON_JWT_SECRET_FILE` | `/etc/amerimon/jwt.secret` | Ruta alternativa al secreto JWT |

Después de editar configuración:

```bash
sudo systemctl restart amerimon
sudo journalctl -u amerimon -n 100 --no-pager
```

No publiques `amerimon.env`, `jwt.secret`, copias de la base ni contraseñas AMI.

## 6. Configuración de Asterisk y nodos

### AMI global

El archivo `/etc/amerimon/amerimon.yaml` define la conexión AMI global:

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

En la instalación Debian, `postinstall.sh` intenta leer el primer usuario AMI que no sea `[general]` en `/etc/asterisk/manager.conf`. Solo agrega `AMI_USER` y `AMI_SECRET` si todavía no están definidos en `amerimon.env`. Si no puede leerlos, Amerimon sigue instalado, pero las consultas y acciones AMI no funcionarán hasta completar esas variables.

Amerimon no modifica `manager.conf` ni crea usuarios Asterisk. El usuario AMI existente debe tener los permisos necesarios para las acciones que se usan, incluida la ejecución de comandos y `Originate` para Listen Live.

Si existe `/etc/asterisk`, el postinstalador guarda una sola copia previa de `modules.conf` (`modules.conf.bak-listen-live`) y habilita los módulos presentes `res_speech.so`, `res_agi.so`, `app_chanspy.so`, `app_mixmonitor.so`, `app_verbose.so`, `func_config.so`, `func_strings.so`, `func_logic.so` y `func_cut.so`. `app_verbose.so` proporciona las aplicaciones `Verbose` y `Log` del contexto: si no está cargado, Asterisk termina la llamada antes de llegar a ChanSpy. Convierte una directiva `noload` de esos módulos en `load` o agrega `load => ...` al final. Si existe `extensions.conf`, guarda `extensions.conf.bak-listen-live` y agrega los contextos `[listen-live-spy]`, `[listen-live-eagi]` y `[amerimon-listen-node]` cuando faltan. Solo migra los bloques `nodo` antiguos reconocidos de Amerimon (SimpleUSB o Local/pseudo), sustituyéndolos por una única redirección al contexto nuevo; los bloques personalizados se conservan y generan un aviso. Antes de una migración se guarda también `extensions.conf.bak-listen-live-rxchannel`.

Si Asterisk ya está activo, el hook intenta cargar esos módulos y ejecutar `dialplan reload`; no reinicia el servicio Asterisk. Si un módulo no existe en esa distribución, registra un aviso y continúa la instalación de Amerimon. Las copias `.bak-listen-live` permiten revisar o restaurar manualmente el estado previo.

### Selección y sintaxis de nodos Allmon

La búsqueda de `allmon.ini` sigue este orden:

1. `sources.allmon_ini_path`, si se configuró explícitamente.
2. `/etc/allmon3/allmon.ini`.
3. `/var/www/html/supermon/allmon.ini`.
4. `sources.allmon_ini_own_path`; predeterminado `/etc/amerimon/allmon.ini`.

Los nodos locales se obtienen de `/etc/asterisk/rpt.conf` (o `ami.rpt_config_path`), incluido el principal identificado por `<nodo> = radio@127.0.0.1/<nodo>,NONE` en `[nodes]`. Se excluyen como servidores los nodos de `privatenodes.txt` y los clasificados como privados (cuatro dígitos): no crean tablas ni opciones del selector de servidores. Sus conexiones siguen apareciendo en la tabla del nodo al que están conectadas, sea el principal u otro servidor. El principal no se elige por orden: si varias entradas no privadas cumplen la regla loopback, se registra el error de ambigüedad. Los nodos locales usan las credenciales AMI globales de Amerimon. Las entradas Allmon dirigidas a loopback o al host AMI global no sustituyen estos nodos ni sus credenciales. Las instancias de servidores remotos conservan su configuración Allmon. Esta selección se comparte entre la tabla, el sondeo de actividad y las acciones sobre nodos.

En el monitor AllStarLink, la tabla del nodo principal aparece primero; las restantes se ordenan por número. La cabecera de cada tabla muestra `número - descripción - IP/hostname`; el principal conserva su etiqueta «Nodo principal» y las remotas no llevan el prefijo «Nodo remoto». La descripción procede de `astdb.txt` y la dirección del host AMI global para los nodos locales o del campo `host` de Allmon para los remotos (sin puerto). Se muestra la IP o el hostname según lo configurado, sin consultas DNS; los datos ausentes se omiten, incluso cuando el servidor está desconectado.

Solo el número del nodo se muestra en negrita; la descripción y la IP/hostname usan peso normal, tanto en el principal como en los remotos.

Cuando existe un `allmon.ini` con nodos remotos válidos, esas instancias proporcionan la conexión AMI de cada servidor remoto. Una entrada típica es:

```ini
[616260]
system=Nodes
host=127.0.0.1
port=5038
user=ami_usuario
passwd=CAMBIAR_ESTE_SECRETO
hideNodeURL=no
menu=yes
```

- El nombre de la sección es el número de nodo local, de hasta siete dígitos.
- `system=Nodes` identifica una instancia de nodo; `menu=no` la excluye.
- `host` y `user` son obligatorios; `port` es opcional y por defecto `5038`.
- El parser acepta `host:puerto` en `host` o un puerto separado.
- Si la contraseña de la instancia queda vacía y host/puerto/usuario coinciden con AMI global, se reutiliza el secreto global.
- El editor de Allmon valida hasta 50 nodos, crea una copia `allmon.ini.amerimon.bak` y solo reescribe las secciones de nodos.
- La casilla **Seguimiento** de **Administración → Nodos Allmon** permite excluir un nodo del monitor, sus consultas de actividad y sus operaciones sin borrarlo. Amerimon guarda esa elección con `amerimon_tracking=disabled` en su stanza; al volver a activar la casilla, elimina únicamente esa opción propia y conserva las demás configuraciones.

Si no hay instancias Allmon configuradas, los nodos locales se leen de `rpt_config_path` (normalmente `/etc/asterisk/rpt.conf`) y todos usan la AMI global.

La configuración Allmon puede consultarse y editarse desde **Administración → Nodos Allmon**. Para que los cambios de `host`, `port`, `user` o `passwd` surtan efecto no se reinicia Asterisk; la siguiente operación de Amerimon vuelve a leer el archivo.

### Conexiones y comprobación

Las acciones integradas usan `rpt cmd` por AMI:

| Acción | Código `ilink` temporal | Código permanente |
| --- | ---: | ---: |
| Transceive / Connect | `3` | `13` |
| Monitor | `2` | `12` |
| Local Monitor | `8` | `18` |
| Disconnect | `11` | No aplica |

Para conexiones temporales, Amerimon consulta `XStat` hasta confirmar el enlace `ESTABLISHED`. El timeout predeterminado es 15 segundos, configurable entre 1 y 300 segundos en la petición. Las permanentes no esperan esa confirmación automática.

Listen Live recibe audio solo del **nodo principal configurado para Asterisk**, identificado por la entrada `<nodo> = radio@127.0.0.1/<nodo>,NONE` en `rpt.conf`. Para la petición `ext=nodo`, Amerimon origina `Local/<nodo>@amerimon-listen-node/n`. El contexto obtiene el último valor efectivo de `rxchannel` mediante `AST_CONFIG(...,rxchannel,-1)`, respetando las plantillas e inclusiones que procesa Asterisk, y ejecuta `ChanSpy` en modo escucha. La ruta configurada en `ami.rpt_config_path` se transmite como variable heredada AMI `__AMERIMON_RPT_CONFIG`.

No se impone SimpleUSB: por ejemplo, `SimpleUSB/616265` y `Radio/616265` se conservan; `Local/pseudo` se normaliza a `Local/pseudo@default`, mientras `Local/pseudo@otro-contexto/n` se normaliza a `Local/pseudo@otro-contexto` para buscar el prefijo del canal activo. No se habilita whisper ni barge. Si falta `rxchannel`, el dialplan registra un error y cuelga, sin espiar un canal arbitrario. Esto no demuestra compatibilidad de audio con todas las tecnologías: cada equipo debe probarse con tráfico real.

El instalador agrega `[amerimon-listen-node]` si falta y recarga el dialplan sin reiniciar Asterisk. El contexto `[listen-live-spy]` se mantiene para la escucha de extensiones PJSIP. Tras actualizar, detén e inicia una escucha nueva: una llamada existente no cambia de aplicación al recargar el dialplan. Si falta la entrada del nodo principal, la API devuelve un error de configuración.

Si se instaló la versión intermedia que generaba el contexto `amerimon-listen-node` con `Rpt(...,Pm)`, el instalador reconoce ese bloque exacto y lo migra a la lectura de `rxchannel`. No reemplaza variantes personalizadas de ese contexto.

Para probar cada instalación Debian 12/13 después de actualizar:

1. Detén la escucha anterior y comprueba `asterisk -rx "dialplan show amerimon-listen-node"`.
2. Inicia Listen Live con tráfico conocido en el nodo. En `asterisk -rx "core show channels concise"` debe aparecer `ChanSpy` con el canal de recepción real, no una tecnología impuesta.
3. Revisa `journalctl -u amerimon -f`: debe aparecer `primer audio recibido`. Comprueba también que el navegador reproduce el tráfico; recibir bloques por sí solo no garantiza contenido audible.
4. Detén la escucha y confirma el resumen con bloques/bytes recibidos, `hangup confirmed` y la ausencia de los canales Local de esa escucha.

No selecciona nodos remotos de Allmon. Su puerto EAGI `8086` permanece enlazado a loopback.

### Diagnosticar y limpiar canales huérfanos

El paquete instala `/usr/local/bin/limpiar_canales_listen_live.sh`. El modo predeterminado `--check` solo muestra canales `Local/...@listen-live-spy` y `Local/...@amerimon-listen-node`, y procesos `audio_eagi.sh`, `cat` y `nc` asociados al listener:

```bash
sudo limpiar_canales_listen_live.sh --check
```

Para solicitar hangup de los canales y terminar esos procesos EAGI:

```bash
sudo limpiar_canales_listen_live.sh --clean
```

La limpieza interrumpe todas las escuchas Listen Live activas y requiere escribir `LIMPIAR`. El script no mata el proceso Asterisk ni su grupo de procesos. Los procesos zombie no se pueden matar directamente; Asterisk debe recogerlos.

## 7. Base de datos, migración y respaldo

En producción, la DB es `/var/lib/amerimon/amerimon.db`; en desarrollo con `make run`, es `/tmp/amerimon-dev.db`. Cambiar `AMERIMON_DB` selecciona otro archivo.

Las migraciones se aplican al abrir la base. Si ya hay migraciones aplicadas, antes de aplicar nuevas se crea un backup con sufijo de fecha y se conservan las tres copias más recientes.

Para una copia manual consistente:

```bash
sudo systemctl stop amerimon
sudo cp -a /var/lib/amerimon/amerimon.db \
  "/var/lib/amerimon/amerimon.db.manual-$(date +%Y%m%d-%H%M%S)"
sudo systemctl start amerimon
```

Para restaurar, detén el servicio, conserva una copia de la DB actual, reemplaza el archivo por el backup y vuelve a iniciarlo. Mantén propietario/permisos root y revisa el estado con `journalctl`.

## 8. Administración y endpoints

En el primer acceso, pulsa **Admin** y define el usuario y contraseña. El nombre debe tener entre 3 y 32 caracteres permitidos por la aplicación; la contraseña requiere al menos 8 caracteres y se guarda con bcrypt. Los endpoints administrativos requieren JWT de rol administrador.

### Restablecer la cuenta de administrador

Si se olvidó la contraseña, ejecuta como usuario con sudo:

```bash
sudo amerimon-reset-password
```

El comando elimina el registro de administrador de la tabla `admin`; no borra la base de datos ni los nodos, favoritos, historial de conexiones o registros Asterisk. Después, abre la web, pulsa **Admin** y vuelve a definir el usuario y la contraseña.

El reset de contraseña no rota `/etc/amerimon/jwt.secret` ni revoca tokens JWT ya emitidos. Si además se necesita invalidar todos los tokens, respalda y regenera el secreto JWT y reinicia el servicio:

```bash
sudo cp -a /etc/amerimon/jwt.secret /etc/amerimon/jwt.secret.bak
sudo sh -c 'umask 077; head -c 48 /dev/urandom | base64 -w0 > /etc/amerimon/jwt.secret'
sudo systemctl restart amerimon
```

El diálogo de acceso permite marcar **Recordar usuario, contraseña y sesión en este dispositivo**. Sin marcarlo, el JWT permanece en memoria y se pierde al recargar; al marcarlo, se almacena en Local Storage hasta cerrar sesión o recibir un rechazo del servidor. La casilla, el último usuario y su contraseña se conservan por separado en Local Storage incluso después de salir y se restauran al abrir el formulario. Un acceso correcto con la casilla desmarcada elimina la preferencia y las credenciales recordadas. Un cambio correcto de contraseña actualiza la recordada. Un acceso fallido no reemplaza las credenciales guardadas. **La contraseña se guarda sin cifrar, accesible a los scripts de la página y a quien acceda al perfil del navegador. Usa esta opción solo en dispositivos de confianza.**

La interfaz utiliza únicamente el tema oscuro, incluidos los formularios y los diagramas del manual. Los títulos, etiquetas y textos deben mantener un contraste mínimo de 4,5:1 (3:1 para texto grande); las reglas oscuras globales conservan su prioridad frente a los estilos encapsulados de Angular. La impresión conserva su formato de papel.

Si hay varios servidores, **Mostrar servidor** permite elegir un nodo local o **Todos**. El selector compacto aparece junto a **Buscar conexiones** en pantallas amplias; en móvil se apilan sin desbordar.

El pie compacto muestra la versión Git de Amerimon y los créditos **Powered by Angular Framework, SQLite3 y Go Language**, con logos locales adaptados al tema oscuro. `make run` y `make dev-backend` inyectan `git describe --tags --always`; los paquetes reciben la versión del tag mediante GoReleaser. Un `go run` directo sin inyectar la versión muestra `dev`. No incluye enlaces legales ni contador de visitas y se oculta al imprimir.

### Enlaces externos

El botón general **Enlaces externos** está disponible para todos los visitantes y muestra únicamente registros activos. Desde **Administrar enlaces**, el administrador puede crear, editar y eliminar registros de `enlaces_externos`, con confirmación de borrado. Se conservan los campos `dashid`, `descripcion`, `url`, `target` e `is_active`; el esquema se adapta a SQLite mediante una migración nueva sin modificar los datos existentes. La descripción y URL admiten hasta 255 caracteres, se aceptan solo URL HTTP/HTTPS y los destinos `_blank` (nueva pestaña) o `_self` (misma pestaña).

El reporte de escuchas de Listen Live ya no muestra las columnas **Usuario** e **Indicativo**. Se conservan ID, IP de origen, inicio y las acciones autorizadas.

| Ruta | Acceso | Función |
| --- | --- | --- |
| `/mon/health` | Público | Salud de HTTP/DB |
| `/mon/auth/login`, `/mon/auth/status`, `/mon/auth/setup` | Público | Inicio y configuración inicial de administrador |
| `/mon/allstarlink/nodes`, `/mon/allstarlink/activity` | Público, token opcional | Nodos y actividad |
| `/mon/listen-live/*` | Público, token opcional según operación | Sesiones, estado y WebSocket de audio |
| `/mon/allmon/config` | Administrador | Leer/guardar nodos Allmon |
| `/mon/asterisk/commands` | Administrador | Listar y ejecutar acciones Asterisk |
| `/mon/asterisk/logs` | Administrador | Últimas 500 operaciones por defecto, máximo 2000 |
| `/mon/allstarlink/connections/history` | Administrador | Historial de conexiones |
| `/mon/system-info` | Administrador | Sistema, CPU, memoria, discos, red, temperatura, voltaje y Asterisk |
| `/mon/site-settings` | GET público; PUT administrador | Título/subtítulo de cabecera |
| `/mon/favoritos` | Administrador | Favoritos |
| `/mon/enlaces-externos` | Público | Solo enlaces activos |
| `/mon/admin/enlaces-externos`, `/mon/admin/enlaces-externos/:id` | Administrador | CRUD de enlaces externos |

## 9. Registro de operaciones Asterisk

Las operaciones del panel AllStarLink se guardan en `asterisk_operation_logs` y aparecen en **Administración → Registros Asterisk**. Se almacenan fecha UTC, acción, nodo local/remoto, modo, comando enviado, resultado, detalle y duración. Los detalles se limitan a 2000 caracteres.

Los resultados distinguen, entre otros, solicitud inválida, error de configuración, error AMI, comando aceptado pero enlace no confirmado y éxito. Los logs de proceso también usan el prefijo `asterisk:` y pueden consultarse con:

```bash
sudo journalctl -u amerimon -f | grep 'asterisk:'
```

No se guardan `AMI_SECRET`, contraseñas ni tokens. El endpoint y la página de registros están restringidos al administrador. La lista ofrece hasta 2000 eventos; no hay purga automática en la aplicación, así que la base debe incluirse en la política de respaldo/retención.

Este historial cubre las acciones del panel de comandos/conexiones y los intentos de inicio/parada de Listen Live (`listen_start`, `listen_stop`), incluidos los errores AMI. Un inicio correcto indica que AMI aceptó `Originate`, no que ya llegue audio. No reemplaza los logs de Asterisk.

### Diagnóstico de Listen Live en Raspberry Pi (Debian 12 y 13)

Después de instalar el paquete, abre **Registros Asterisk** para consultar los intentos de escucha. Para seguir el recorrido completo, ejecuta por SSH antes de pulsar **LISTEN LIVE**:

```bash
sudo journalctl -u amerimon -f -o short-iso
```

Se registran la solicitud de inicio, la aceptación o rechazo AMI, la conexión WebSocket, la conexión EAGI, el primer bloque de audio y el total de bloques/bytes al terminar. El UUID permite correlacionar las etapas sin registrar credenciales ni contenido de audio. Si aparece `EOF`, el mensaje identifica si se cerró la conexión durante el banner, login u `Originate`; un login rechazado se muestra como tal, sin intentar originar después.

Para recoger evidencia de una prueba fallida y copiarla desde la Raspberry:

```bash
sudo journalctl -u amerimon --since "15 minutes ago" -o short-iso --no-pager > /tmp/amerimon-diagnostico.log
sudo journalctl -u asterisk --since "15 minutes ago" -o short-iso --no-pager > /tmp/asterisk-diagnostico.log
uname -m
dpkg-query -W amerimon asterisk
sudo ss -ltnp '( sport = :8082 or sport = :8086 )'
```

Si Asterisk escribe en archivo en lugar del journal, consulta también `/var/log/asterisk/messages.log`. Los mensajes de arranque muestran la versión instalada. No compartas `amerimon.env`, `jwt.secret` ni `manager.conf`, ya que contienen credenciales.

## 10. Información del sistema

### Mantenimiento administrativo

En **Admin → Sistema**, el administrador dispone de **Reiniciar Asterisk**, **Reiniciar Linux** y **Apagar Linux**. Cada acción exige escribir su frase de confirmación; todas interrumpen comunicaciones. La API `POST /mon/system/control` exige JWT y rol administrador y solo acepta estas tres acciones fijas, nunca comandos arbitrarios.

- **Asterisk:** intenta `systemctl restart asterisk.service`. Si falla, solicita la parada del servicio y envía SIGTERM solo a los procesos de esa unidad; si no terminan, usa SIGKILL. No busca procesos por nombre ni termina procesos ajenos. Solo hace `start` tras confirmar la parada; después verifica servicio activo y AMI mediante uptime. Una recuperación o verificación fallida se devuelve como error, sin informar un reinicio exitoso. El plazo total es aproximadamente un minuto.
- **Diagnóstico:** muestra los errores del journal de `asterisk.service` desde la solicitud y los errores recientes del archivo estándar `/var/log/asterisk/messages.log` (últimos 256 KiB, hasta 40 líneas). Si alguna fuente no puede consultarse, lo indica. Los errores del arranque se muestran como advertencias aunque servicio y AMI respondan; por ejemplo, puede faltar una interfaz USB. Un archivo de log personalizado debe consultarse aparte.
- **Linux:** programa `shutdown -r +1` o `shutdown -P +1`, con un minuto de margen. La respuesta confirma la programación, no que el reinicio/apagado haya terminado. El apagado puede requerir acceso físico para volver a encender el equipo. Para cancelar desde una consola antes de que ocurra: `sudo shutdown -c`.

Las solicitudes y sus resultados quedan en **Registros Asterisk** y en el journal de Amerimon. El servicio instalado se ejecuta como root; una instancia de desarrollo sin esos permisos devolverá el error del sistema, sin intentar elevar privilegios. No se deben probar las acciones Linux sobre un equipo compartido o en producción sin autorización. Los tests de estas acciones simulan todos los comandos, sin reiniciar ni apagar el equipo.

El popup **Sistema** requiere sesión de administrador. Incluye:

- Host, distribución, arquitectura, kernel y uptime.
- CPU, carga, memoria, almacenamiento y swap.
- Interfaces y tráfico de red.
- Procesos con mayor uso de CPU.
- Temperatura leída de `/sys/class/thermal/thermal_zone*`.
- Voltaje: `vcgencmd measure_volts core` en ARM; lectura `hwmon` en AMD.
- Versión Asterisk obtenida con `core show version` por AMI.

Temperatura, voltaje o versión pueden aparecer como no disponibles si el kernel, el hardware o AMI no exponen esos datos.

## 11. Apache, Nginx y acceso web

El postinstalador intenta detectar el servidor web activo. Si configura el proxy correctamente, basta con `http(s)://<host>/amerimon/`. No instala certificados TLS: HTTPS solo funciona si el host ya tiene SSL configurado.

### Apache

La instalación habilita `proxy`, `proxy_http`, `proxy_wstunnel`, `headers` y `alias`, activa `conf-available/amerimon.conf`, ejecuta `apache2ctl configtest` y solo recarga/inicia Apache si la prueba pasa. El proxy cubre HTTP/HTTPS según el VirtualHost que reciba la petición, incluidas las cabeceras de prefijo y el WebSocket.

Comprobaciones:

```bash
sudo apache2ctl configtest
sudo apache2ctl -S
ls -l /etc/apache2/conf-enabled/amerimon.conf
curl -i http://127.0.0.1/amerimon/mon/health
```

Un aviso `AH00558` sobre `ServerName` no equivale a fallo si `configtest` termina con `Syntax OK`.

### Nginx

El paquete incluye `/usr/lib/amerimon/configure-nginx.sh` y un snippet en `/usr/share/amerimon/nginx-amerimon.conf.example`. El helper busca el VirtualHost HTTP predeterminado (o el primero con `listen 80`), inserta allí el `include`, ejecuta `nginx -t` y revierte el archivo si la prueba o recarga falla.

El snippet define `/amerimon/`, el WebSocket y las cabeceras del proxy. Nginx debe tener un bloque `server` HTTP válido. Si el sitio usa un bloque TLS separado en `443`, verifica que el `include /etc/nginx/snippets/amerimon.conf;` también esté dentro de ese bloque para que funcione por HTTPS. El paquete no crea ni renueva certificados.

Comprobaciones:

```bash
sudo nginx -t
sudo nginx -T | grep -n -A8 -B4 amerimon
curl -i http://127.0.0.1/amerimon/mon/health
```

Si no hay Apache ni Nginx, el paquete no debe fallar por eso: la aplicación queda en `:8082`, sin ruta `/amerimon` ni TLS.

## 12. Actualización AstDB

`amerimon-astdb.timer` se habilita en la instalación y ejecuta `astdb.sh` cada cinco minutos, con retardo aleatorio de arranque. El script descarga `https://allmondb.allstarlink.org/allmondb.php`, combina `privatenodes.txt` si existe y escribe `astdb.txt` en el directorio de Supermon si existe; en caso contrario, usa `/var/lib/asterisk/`.

```bash
systemctl status amerimon-astdb.timer
systemctl list-timers amerimon-astdb.timer
sudo systemctl start amerimon-astdb.service
sudo journalctl -u amerimon-astdb.service -n 50 --no-pager
```

El YAML debe apuntar `sources.astdb_path` al archivo que realmente se actualiza.

## 13. Desarrollo local (solo desarrolladores)

Antes de iniciar el modo desarrollo, detén el servicio instalado para liberar `8082` y `8086`:

```bash
sudo systemctl stop amerimon
make run
```

`make run` compila Angular, lo incrusta y ejecuta Go con `127.0.0.1:8082`, DB `/tmp/amerimon-dev.db` y secreto JWT `dev-secret`. Usa las credenciales de `/etc/asterisk/manager.conf` y `packaging/amerimon.yaml`. Termina con `Ctrl+C`.

Para alternar al servicio instalado con el código actual:

```bash
./install.sh
```

`install.sh` recompila frontend/backend, actualiza `/usr/bin/amerimon`, instala el script EAGI, la migración del dialplan y los archivos auxiliares del servicio. Ejecuta el mismo `packaging/postinstall.sh` del paquete Debian para migrar Listen Live con respaldo, habilitar los módulos disponibles, recargar el dialplan y reiniciar Amerimon aunque ya esté activo. Conserva los archivos existentes de configuración, credenciales y Allmon. También aplica la configuración del proxy web y del temporizador de astdb del paquete. No ejecutes `make run` y `amerimon.service` simultáneamente porque comparten puertos.

Para recarga separada de frontend y backend, utiliza dos terminales: `make dev-backend` y `npm start`.

## 14. Construcción y publicación (solo desarrolladores)

Esta sección es exclusivamente para el equipo que desarrolla y publica Amerimon. **No aplica a clientes**: para instalar o actualizar, usar APT como se indica en la sección 4. `make dist` exige un árbol Git limpio y publica de forma real. Ejecuta:

1. Calcula/sube el siguiente tag semver.
2. Corre `goreleaser release --clean`; sus hooks ejecutan `go mod tidy`, `npm ci` y `npm run build`.
3. Compila `.deb` para Linux `amd64` y `arm64` con CGO desactivado.
4. Copia paquetes al repositorio local, conserva hasta tres versiones por arquitectura, regenera/firma índices y hace push a `jjsainzc/repo-deb`.

Comandos relacionados:

```bash
make dist
make dist VERSION=v1.2.0
make index-repo
make push-repo
```

Antes de entregar instrucciones APT, comprobar que `https://jjsainzc.github.io/repo-deb/Packages` incluya la versión publicada. En los clientes, confirmar con `apt-cache policy amerimon` que el `Candidate` coincida.

## 15. Diagnóstico rápido

| Síntoma | Comprobaciones |
| --- | --- |
| Servicio no inicia | `systemctl status amerimon`; `journalctl -u amerimon -n 100`; revisar `8082` y `8086` |
| Nodo local no aparece | Confirmar `rpt_config_path`, las secciones numéricas y la entrada loopback del nodo principal en `rpt.conf`; Allmon solo aporta servidores remotos |
| AMI rechaza operación | Revisar `AMI_USER`/`AMI_SECRET` sin imprimirlos; validar host/puerto y permisos del usuario existente; consultar `journalctl -u amerimon` |
| AMI acepta, pero no confirma conexión | Abrir Registros Asterisk y revisar detalle de `XStat`, estado remoto y timeout |
| `/amerimon` devuelve 404 | Confirmar conf Apache/Nginx habilitada, ruta, `configtest` y proxy del VirtualHost correcto |
| `apt` ofrece versión anterior | Ejecutar `apt update`, revisar `apt-cache policy amerimon` y confirmar el índice público de Pages |
| Temperatura/voltaje no disponible | Verificar sensores del kernel, `vcgencmd` en ARM o entradas `hwmon` en AMD |

No se debe editar Asterisk para resolver un error del proxy web; tampoco exponer el puerto AMI `5038` fuera del host sin un diseño de seguridad específico.
