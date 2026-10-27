# Amerimon

Interfaz web para consultar AllStarLink, administrar nodos Allmon y escuchar el nodo principal de Asterisk.

Al agregar o editar favoritos, la búsqueda en AstDB se ejecuta con el botón de lupa o con Enter, no mientras se escribe. Si hay más de 50 coincidencias, se muestran solo las primeras 50 y un aviso para precisar la búsqueda, evitando renderizar listas demasiado grandes.

Los campos de IP, nodo e indicativo y sus valores en tablas y vistas incluyen un icono para copiar el valor actual al portapapeles, sin enviar formularios ni abrir los enlaces. No se incluyen contraseñas ni secretos AMI. En nodos privados se copia la línea completa del editor. El control confirma «Copiado» o muestra un error si el navegador bloquea la operación. En HTTP sin API de portapapeles se intenta la copia compatible del navegador; si tampoco está permitida, se indica copiar manualmente.

El monitor consulta cada servidor del grupo de forma independiente. Si un servidor no responde, reintenta su conexión en segundo plano cada 5 segundos sin bloquear los demás, incluso con la actualización automática desactivada. Los fallos de consulta de nodos, incluido el principal, no muestran mensajes de error: su grupo indica únicamente que está reintentando en segundo plano. Los errores al obtener la configuración del monitor y los de operaciones solicitadas por el usuario siguen mostrándose.

La comprobación de topología utiliza los nodos que responden con una lista interpretable y las conexiones pendientes conocidas. Si otros nodos no responden o no se pueden consultar, no bloquea la conexión ni muestra un error de validación de topología; el diagnóstico queda en el log del servidor. Los autoenlaces y las rutas que sí se detectan siguen bloqueados. Con información incompleta no se puede garantizar la ausencia de bucles.

El selector de servidor local permanece sincronizado con el nodo utilizado en las conexiones y operaciones Asterisk, incluso al cargar o actualizar la lista de servidores. La confirmación conserva los parámetros mostrados: cambios posteriores del formulario no alteran la solicitud confirmada. Si el servidor deja de estar disponible en la lista antes de confirmar, la operación se bloquea con un aviso.

El panel **Conectar nodo remoto** conserva el servidor local, nodo remoto, modo, permanencia, desconexión previa y timeout al navegar a otra pantalla y regresar, aunque todavía no se haya enviado el formulario. Estos valores se mantienen en memoria hasta recargar la aplicación. Al regresar se comprueba que el servidor recordado siga disponible; si ya no existe, se selecciona el nodo principal disponible o se deja el selector vacío. Recuperar los valores no ejecuta ninguna operación.

Al conectar un nodo que ya tiene un enlace directo en el servidor seleccionado, Amerimon compara el modo actual y la permanencia del último comando de conexión registrado. Si coinciden con lo solicitado, avisa sin enviar comandos. Si difieren, desconecta únicamente ese enlace (incluidos los permanentes), espera a que XStat confirme su desaparición y vuelve a comprobar la protección contra bucles antes de conectarlo con la nueva modalidad. Si falla la desconexión o no se confirma en 15 segundos, no reconecta. XStat no informa la permanencia: para enlaces sin un registro de conexión compatible, por ejemplo creados fuera de Amerimon, la confirmación advierte que se reconectará para aplicar los valores solicitados. Los cambios externos de permanencia no son detectables por XStat.

En **Conectar nodo remoto**, la casilla **Desconectar todos antes de conectar** está desmarcada por defecto. Si se activa, el componente consulta la lista actualizada del servidor local seleccionado y envía una desconexión individual por cada enlace RPT, incluidos los permanentes. Después vuelve a consultar hasta comprobar que no quedan enlaces y solo entonces solicita la nueva conexión con el nodo, modo, permanencia y timeout confirmados. No afecta a los demás servidores ni utiliza el filtro visual del monitor. Si una consulta o desconexión falla, o la lista no queda vacía en los 15 segundos posteriores a las desconexiones, no se inicia la nueva conexión y se muestra un error; los enlaces ya desconectados no se restauran automáticamente.

## Instalación

### Silenciar estaciones del servidor principal

El administrador dispone de una bocina junto al botón de desconexión de cada enlace RPT del nodo principal. Permite bloquear el audio que esa estación envía al servidor mediante AMI `MuteAudio`, `Direction: in`, manteniendo su conexión y su escucha. No libera necesariamente un PTT bloqueado ni controla las conexiones de la estación con otros servidores.

El postinstall incorpora `res_mutestream.so` a `/etc/asterisk/modules.conf` si está instalado, conservando una copia de seguridad y eliminando directivas `noload` contradictorias. No reinicia Asterisk. El usuario AMI necesita permiso para `MuteAudio` (clase `system`) y las consultas de canales. No se habilita el módulo en servidores remotos.

El botón queda deshabilitado si falta soporte o no puede asociarse de forma inequívoca el enlace con su canal. Una bocina tachada indica el último mute confirmado por Asterisk; el borde discontinuo indica estado desconocido, no audio necesariamente habilitado. Los estados se guardan en memoria del backend por identificador de conexión, se pierden al reiniciar Amerimon y no detectan cambios hechos por herramientas externas. Una reconexión requiere volver a aplicar el mute. La aplicación nunca silencia automáticamente ni cambia el icono a un estado confirmado antes de recibir la aceptación AMI.

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

### Usuarios IAX y extensiones PJSIP

En **Admin → Más opciones → Usuarios IAX / Extensiones PJSIP** se abren páginas de administración con listado, creación, edición y eliminación. Los formularios conservan los datos cuando falla una operación y muestran el diagnóstico del backend; **Recargar** permite reintentar la consulta.

IAX utiliza `iax.conf` junto al archivo configurado en `ami.rpt_config_path`; PJSIP utiliza `ami.pjsip_config_path`. El usuario del servicio Amerimon necesita lectura y escritura sobre estos archivos y su directorio para crear el respaldo `.amerimon.bak` y reemplazar el contenido de forma atómica, conservando propietario y permisos. Las recargas se envían por AMI (`iax2 reload` / `pjsip reload`), con las credenciales ya configuradas, no ejecutando Asterisk como root. Si falla la recarga después de guardar, se informa explícitamente: revisa AMI y los registros de Asterisk antes de reintentar.

PJSIP agrupa cada endpoint con sus referencias `auth` y `aors`, aunque los tres objetos tengan nombres distintos, y conserva opciones no expuestas en los formularios. Para editar o eliminar se requiere un único objeto auth `userpass` y un único AOR, exclusivos del endpoint y presentes en el archivo principal. Las plantillas, los objetos compartidos y las configuraciones ambiguas se rechazan sin modificarlos; los archivos incluidos mediante `#include` no se recorren ni se editan, y se bloquean las operaciones sobre secciones que contienen estas directivas. Antes de trabajar con una configuración heredada o distribuida, revísala manualmente.

Abre `http(s)://<host>/amerimon/` con el proxy configurado, o `http://<IP-del-servidor>:8082/` si accedes directamente. Pulsa **Admin** y define el usuario y contraseña iniciales. El nombre de usuario debe tener entre 3 y 32 caracteres; la contraseña debe tener al menos 8. La contraseña se guarda como hash.

Las advertencias y confirmaciones permiten marcar **No volver a mostrar este tipo de aviso**, por tipo y para este navegador, incluso tras recargar o cerrar sesión. En las confirmaciones, la preferencia solo se guarda al confirmar, no al cancelar; las siguientes acciones de ese tipo se ejecutan sin otra confirmación. Los errores y los diagnósticos de operaciones fallidas siempre se muestran. Temporalmente, **Admin → Personalizar cabecera → Restablecer advertencias y confirmaciones** recupera todos los avisos sin cambiar la cabecera ni la sesión. Consulta los detalles en los manuales técnicos [español](public/assets/docs/technical-operations-manual.es.md) e [inglés](public/assets/docs/technical-operations-manual.en.md).

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

Para configuración de nodos, puertos, SQLite, proxy, respaldos, operaciones administrativas y diagnóstico, consulta el [Manual de Operación Técnica](public/assets/docs/technical-operations-manual.es.md).
