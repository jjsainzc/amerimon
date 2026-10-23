# Amerimon

Interfaz web para consultar AllStarLink, administrar nodos Allmon y escuchar el nodo principal de Asterisk.

El monitor consulta cada servidor del grupo de forma independiente. Si un servidor no responde, reintenta su conexión en segundo plano cada 5 segundos sin bloquear los demás, incluso con la actualización automática desactivada. El aviso superior de conexión corresponde únicamente al nodo principal; los errores de otros servidores aparecen en su propio grupo. Los errores al obtener la configuración del monitor se muestran como errores generales.

El selector de servidor local permanece sincronizado con el nodo utilizado en las conexiones y operaciones Asterisk, incluso al cargar o actualizar la lista de servidores. La confirmación conserva los parámetros mostrados: cambios posteriores del formulario no alteran la solicitud confirmada. Si el servidor deja de estar disponible en la lista antes de confirmar, la operación se bloquea con un aviso.

## Instalación

Paquetes Debian disponibles para `arm64` y `amd64`:

```bash
curl -fsSL https://raw.githubusercontent.com/jjsainzc/repo-deb/main/hc1lnm.gpg \
  | sudo gpg --dearmor --yes -o /usr/share/keyrings/hc1lnm-archive-keyring.gpg

echo "deb [signed-by=/usr/share/keyrings/hc1lnm-archive-keyring.gpg arch=$(dpkg --print-architecture)] https://jjsainzc.github.io/repo-deb/ ./" \
  | sudo tee /etc/apt/sources.list.d/amerinet.list

sudo apt update
sudo apt install amerimon
```

El instalador habilita `amerimon.service` y, si detecta Apache o Nginx, intenta configurar el proxy para `/amerimon`. Sin servidor web, abre `http://<IP-del-servidor>:8082/`.

El paquete `.deb` declara `libc-bin`, `libnss-mdns` y `avahi-daemon` como dependencias para resolver servidores `nombre.local` mediante mDNS sin depender de cgo. APT instala esas dependencias automáticamente. `avahi-utils` es opcional para diagnóstico; no es necesario para ejecutar Amerimon. Para instalar un `.deb` descargado y resolver sus dependencias, usa `sudo apt install ./amerimon_<version>_<arquitectura>.deb`, no solamente `dpkg -i`.

El servidor remoto debe publicar su nombre y la red debe permitir mDNS. Consulta [Usar hostname en la LAN](docs/hostname-lan.md). `install.sh` es una herramienta de instalación desde fuentes para desarrollo, no el mecanismo de distribución a otros equipos.

## Primer acceso

Abre `http(s)://<host>/amerimon/` con el proxy configurado, o `http://<IP-del-servidor>:8082/` si accedes directamente. Pulsa **Admin** y define el usuario y contraseña iniciales. El nombre de usuario debe tener entre 3 y 32 caracteres; la contraseña debe tener al menos 8. La contraseña se guarda como hash.

### Ubicación del clima

Cualquier visitante puede pulsar **Cambiar ubicación**, junto a la localidad consultada, para **Obtener mi ubicación**, volver a **Usar configuración** (solo si el administrador guardó una ubicación) o escribir una **Ubicación**. El clima se actualiza automáticamente cada cinco minutos; el botón de recarga junto a la localidad permite actualizarlo manualmente. La selección solo afecta al clima de la página actual: no guarda ni modifica la configuración del administrador y se descarta al recargar. La geolocalización requiere HTTPS (o localhost) y autorización del navegador; si el permiso fue denegado previamente, hay que habilitarlo en los permisos del sitio.

## Restablecer contraseña de administrador

Si se olvidó la contraseña, ejecuta en el servidor:

```bash
sudo amerimon-reset-password
```

El comando borra únicamente la cuenta de administrador para que puedas definirla de nuevo desde **Admin**. No borra nodos Allmon, favoritos, registros ni el resto de la base de datos. Después, vuelve a abrir la web y define el usuario y la contraseña.

## Diagnosticar Listen Live

Para consultar canales o procesos EAGI residuales sin interrumpir escuchas:

```bash
sudo limpiar_canales_listen_live.sh --check
```

La opción `--clean` solicita hangup y termina procesos EAGI huérfanos, pero interrumpe todas las escuchas activas y pide confirmación escribiendo `LIMPIAR`.

## Servicio y actualización

```bash
sudo systemctl status amerimon --no-pager
sudo apt update
apt list --upgradable
sudo apt upgrade
```

## Depuración en VS Code

Consulta la [guía paso a paso de depuración de Angular y Go](docs/depuracion-vscode.md).

1. Instala las dependencias con `npm ci` si aún no están instaladas y asegúrate de tener Go (la versión indicada en `go.mod`).
2. Instala la extensión recomendada **Go** (`golang.go`) en el entorno donde abres el proyecto (también en SSH/WSL, si corresponde). Si VS Code solicita instalar **Delve**, acepta.
3. Libera los puertos `4200`, `8082` y `8086`: detén otras ejecuciones de desarrollo. Si está activo el servicio instalado, usa `sudo systemctl stop amerimon`.
4. En **Ejecutar y depurar**, selecciona **Angular + Go** y pulsa **F5**.

El navegador abre `http://localhost:4200/`. Angular reenvía las peticiones `/mon` al backend Go en `127.0.0.1:8082`. Puedes poner breakpoints tanto en TypeScript como en Go. La configuración **Backend Go** permite depurar solo el backend; **ng serve** solo inicia Angular.

La depuración usa `packaging/amerimon.yaml`, la base SQLite `/tmp/amerimon-dev.db` y un secreto JWT exclusivo de desarrollo (`dev-secret`). No uses este secreto en producción. Las rutas de Asterisk y Allmon siguen siendo las del YAML: las acciones administrativas pueden modificar archivos y nodos reales.

Antes de iniciar Go, una tarea prepara las credenciales AMI: usa `AMI_USER` y `AMI_SECRET` del entorno de VS Code o, si faltan, busca una cuenta con contraseña en `/etc/asterisk/manager.conf`. Las guarda en `.vscode/ami.env`, excluido de Git y con permisos `0600`. Si no puede leer la configuración o encontrar una cuenta, la tarea falla con un mensaje explícito. No guardes credenciales en `launch.json`.

**Angular + Go** ejecuta esa tarea automáticamente antes de resolver las configuraciones del depurador. Para iniciar **Backend Go** por separado, ejecuta primero **Tareas: Ejecutar tarea → Preparar AMI para depurar**.

**Detener** finaliza las sesiones de depuración de Angular y Go. La tarea `npm: start` puede seguir activa: termínala con **Tareas: Terminar tarea** antes de volver a iniciarla. Si detuviste el servicio instalado, restáuralo al terminar con `sudo systemctl start amerimon`.

## Manual técnico

Para configuración de nodos, puertos, SQLite, proxy, respaldos, operaciones administrativas y diagnóstico, consulta el [Manual de Operación Técnica](docs/manual-operacion-tecnica.md).
