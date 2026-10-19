# Manual de Operación Técnica de Amerimon

## 1. Alcance

Amerimon es una aplicación web para consultar AllStarLink, administrar nodos Allmon, escuchar audio en vivo y ejecutar operaciones autorizadas en Asterisk. El frontend Angular se compila y se incrusta en un único servicio Go.

Este manual describe el árbol de código actual. La versión Debian publicada de referencia es `1.0.4`. Los clientes instalan y actualizan Amerimon con APT; no necesitan compilar el código ni ejecutar tareas de publicación.

Amerimon consulta Asterisk mediante AMI. No reemplaza ni reconfigura IAX, PJSIP o AllStarLink; su postinstalador prepara únicamente los módulos y contextos necesarios para Listen Live.

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
| `/var/lib/amerimon/amerimon.db` | Base SQLite de producción |
| `/lib/systemd/system/amerimon.service` | Unidad systemd |
| `/usr/local/bin/astdb.sh` | Actualización de la base de nodos AstDB |

Las configuraciones `amerimon.env` y `amerimon.yaml` se instalan como `config|noreplace`; una actualización conserva los archivos locales existentes.

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

Si existe `/etc/asterisk`, el postinstalador guarda una sola copia previa de `modules.conf` (`modules.conf.bak-listen-live`) y habilita los módulos presentes `res_speech.so`, `res_agi.so`, `app_chanspy.so` y `app_mixmonitor.so`. Convierte una directiva `noload` de esos módulos en `load` o agrega `load => ...` al final. Si existe `extensions.conf`, guarda `extensions.conf.bak-listen-live` y agrega los contextos `[listen-live-spy]` y `[listen-live-eagi]` solo cuando faltan; no reemplaza contextos completos que ya existan.

Si Asterisk ya está activo, el hook intenta cargar esos módulos y ejecutar `dialplan reload`; no reinicia el servicio Asterisk. Si un módulo no existe en esa distribución, registra un aviso y continúa la instalación de Amerimon. Las copias `.bak-listen-live` permiten revisar o restaurar manualmente el estado previo.

### Selección y sintaxis de nodos Allmon

La búsqueda de `allmon.ini` sigue este orden:

1. `sources.allmon_ini_path`, si se configuró explícitamente.
2. `/etc/allmon3/allmon.ini`.
3. `/var/www/html/supermon/allmon.ini`.
4. `sources.allmon_ini_own_path`; predeterminado `/etc/amerimon/allmon.ini`.

Cuando existe un `allmon.ini` con nodos válidos, esas instancias proporcionan la conexión AMI de cada nodo. Una entrada típica es:

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

Listen Live recibe audio solo del **nodo principal configurado para Asterisk**. No selecciona nodos remotos de Allmon. Su puerto EAGI `8086` permanece enlazado a loopback.

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

El frontend mantiene el JWT en memoria; una recarga de página puede requerir iniciar sesión de nuevo.

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

## 9. Registro de operaciones Asterisk

Las operaciones del panel AllStarLink se guardan en `asterisk_operation_logs` y aparecen en **Administración → Registros Asterisk**. Se almacenan fecha UTC, acción, nodo local/remoto, modo, comando enviado, resultado, detalle y duración. Los detalles se limitan a 2000 caracteres.

Los resultados distinguen, entre otros, solicitud inválida, error de configuración, error AMI, comando aceptado pero enlace no confirmado y éxito. Los logs de proceso también usan el prefijo `asterisk:` y pueden consultarse con:

```bash
sudo journalctl -u amerimon -f | grep 'asterisk:'
```

No se guardan `AMI_SECRET`, contraseñas ni tokens. El endpoint y la página de registros están restringidos al administrador. La lista ofrece hasta 2000 eventos; no hay purga automática en la aplicación, así que la base debe incluirse en la política de respaldo/retención.

Este historial cubre las acciones del panel de comandos/conexiones; no es una captura de todos los eventos internos ni reemplaza los logs de Asterisk.

## 10. Información del sistema

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

## 13. Desarrollo local (solo mantenedores)

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

`install.sh` recompila frontend/backend, actualiza `/usr/bin/amerimon` y activa el servicio systemd. Si se ejecuta cuando ya está activo, `systemctl enable --now` no reinicia un proceso activo; primero detenlo o reinícialo después. No ejecutes `make run` y `amerimon.service` simultáneamente porque comparten puertos.

Para recarga separada de frontend y backend, utiliza dos terminales: `make dev-backend` y `npm start`.

## 14. Construcción y publicación (solo mantenedores)

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
| Nodo local no aparece | Confirmar `rpt_config_path` o la sección numérica `system=Nodes` y `menu=yes` en el `allmon.ini` activo |
| AMI rechaza operación | Revisar `AMI_USER`/`AMI_SECRET` sin imprimirlos; validar host/puerto y permisos del usuario existente; consultar `journalctl -u amerimon` |
| AMI acepta, pero no confirma conexión | Abrir Registros Asterisk y revisar detalle de `XStat`, estado remoto y timeout |
| `/amerimon` devuelve 404 | Confirmar conf Apache/Nginx habilitada, ruta, `configtest` y proxy del VirtualHost correcto |
| `apt` ofrece versión anterior | Ejecutar `apt update`, revisar `apt-cache policy amerimon` y confirmar el índice público de Pages |
| Temperatura/voltaje no disponible | Verificar sensores del kernel, `vcgencmd` en ARM o entradas `hwmon` en AMD |

No se debe editar Asterisk para resolver un error del proxy web; tampoco exponer el puerto AMI `5038` fuera del host sin un diseño de seguridad específico.
