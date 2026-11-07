# Amerimon

Interfaz web para consultar AllStarLink, administrar nodos Allmon y escuchar el nodo principal de Asterisk.

Al agregar o editar favoritos, la búsqueda en AstDB se ejecuta con el botón de lupa o con Enter, no mientras se escribe. Si hay más de 50 coincidencias, se muestran solo las primeras 50 y un aviso para precisar la búsqueda, evitando renderizar listas demasiado grandes.

Los campos de IP, nodo e indicativo y sus valores en tablas y vistas incluyen un icono para copiar el valor actual al portapapeles, sin enviar formularios ni abrir los enlaces. No se incluyen contraseñas ni secretos AMI. En nodos privados se copia la línea completa del editor. El control confirma «Copiado» o muestra un error si el navegador bloquea la operación. En HTTP sin API de portapapeles se intenta la copia compatible del navegador; si tampoco está permitida, se indica copiar manualmente.

El monitor consulta cada servidor del grupo de forma independiente. Si un servidor no responde, reintenta su conexión en segundo plano cada 5 segundos sin bloquear los demás, incluso con la actualización automática desactivada. Los fallos de consulta de nodos, incluido el principal, no muestran mensajes de error: su grupo indica únicamente que está reintentando en segundo plano. Los errores al obtener la configuración del monitor y los de operaciones solicitadas por el usuario siguen mostrándose.

La comprobación de topología utiliza los nodos que responden con una lista interpretable y las conexiones pendientes conocidas. Si otros nodos no responden o no se pueden consultar, no bloquea la conexión ni muestra un error de validación de topología; el diagnóstico queda en el log del servidor. Los autoenlaces y las rutas que sí se detectan siguen bloqueados. Con información incompleta no se puede garantizar la ausencia de bucles.

El selector de servidor local permanece sincronizado con el nodo utilizado en las conexiones y operaciones Asterisk, incluso al cargar o actualizar la lista de servidores. La confirmación conserva los parámetros mostrados: cambios posteriores del formulario no alteran la solicitud confirmada. Si el servidor deja de estar disponible en la lista antes de confirmar, la operación se bloquea con un aviso.

El panel **Conectar nodo remoto** conserva el servidor local, nodo remoto, modo, permanencia, desconexión previa y timeout al navegar a otra pantalla y regresar, aunque todavía no se haya enviado el formulario. Estos valores se mantienen en memoria hasta recargar la aplicación. Al regresar se comprueba que el servidor recordado siga disponible; si ya no existe, se selecciona el nodo principal disponible o se deja el selector vacío. Recuperar los valores no ejecuta ninguna operación.

Al conectar un nodo que ya tiene un enlace directo en el servidor seleccionado, Amerimon compara el modo actual y la permanencia del último comando de conexión registrado. Si coinciden con lo solicitado, avisa sin enviar comandos. Si difieren, desconecta únicamente ese enlace (incluidos los permanentes), espera a que XStat confirme su desaparición y vuelve a comprobar la protección contra bucles antes de conectarlo con la nueva modalidad. Si falla la desconexión o no se confirma en 15 segundos, no reconecta. XStat no informa la permanencia: para enlaces sin un registro de conexión compatible, por ejemplo creados fuera de Amerimon, la confirmación advierte que se reconectará para aplicar los valores solicitados. Los cambios externos de permanencia no son detectables por XStat.

**Administración → Gestión de audios** administra una biblioteca persistente de anuncios para AllStarLink por IAX/IAXRPT. Acepta WAV, MP3, FLAC, OGG/Opus, M4A/MP4, AAC, WMA, AIFF, WebM, MKA, AMR y APE; convierte en el servidor a WAV PCM mono de 16 bits y 8000 Hz mediante FFmpeg. Entrada y resultado tienen un límite de 25 MiB. El instalador local requiere `ffmpeg` y el paquete Debian lo incluye como dependencia. Los audios y sus activaciones permanecen en el servidor aunque el administrador cierre sesión o la web; solo se eliminan por la acción explícita **Eliminar**. Las emisiones por intervalo pueden ser varias, pero se serializan para no superponerse. La emisión programada solo se intenta con silencio total confirmado en todos los nodos monitorizados.

La tarjeta **Grabaciones del nodo principal** captura la escucha del nodo y sus estaciones conectadas en el servidor: iniciar, pausa manual, continuar y detener/guardar. Omite silencios automáticamente y reanuda al detectar audio sin anular una pausa manual. Permite escuchar solo en el navegador, borrar con confirmación o añadir una copia independiente a la biblioteca para emitirla o programarla después. Reutiliza EAGI/Listen Live; no modifica configuraciones de Asterisk. La captura sigue sin la web y se limita a 25 MiB por archivo; tras una interrupción del servicio se recupera el WAV, pero no se reanuda la captura sola.

La pantalla Angular es un módulo reutilizable en `src/app/features/audio-management`. Para integrarlo en otra app se proporciona la URL del API, un traductor para las claves `AUDIO_*` y una ruta protegida por la autorización de administrador del proyecto. Consulta el [README del módulo](src/app/features/audio-management/README.md) y el [manual técnico](public/assets/docs/technical-operations-manual.es.md).

Los botones de audio usan iconos con descripciones y etiquetas accesibles ES/EN. En la biblioteca, los auriculares permiten escuchar localmente sin emitir ni cambiar la programación; el icono de emisión reproduce en Asterisk los audios activados de emisión única. Grabación y escucha local incluyen medidores RMS en dBFS. Durante las solicitudes se bloquean las otras acciones y el proceso aparece solo dentro del botón correspondiente, incluido el porcentaje de subida.

La configuración de fragmento recuerda el inicio y el final por audio, tanto para emisiones manuales como para intervalos, sin modificar el archivo original. Una sola línea de tiempo muestra ambos extremos y resalta la selección; también permite escribir los segundos de inicio y final. **Escuchar selección** incorpora un reproductor con medidor dentro del cuadro, sin guardar ni transmitir a Asterisk. El disquete guarda los límites sin emitir y la tarjeta muestra **Fragmento seleccionado**, con inicio, final y duración. **Restablecer audio completo** y guardar elimina la selección recordada. Los auriculares de la tarjeta y la descarga siguen ofreciendo el audio completo. El servidor genera un WAV temporal para cada emisión del fragmento. El diálogo, validaciones y manuales están disponibles en español e inglés.

En **Conectar nodo remoto**, la casilla **Desconectar todos antes de conectar** está desmarcada por defecto. Si se activa, el componente consulta la lista actualizada del servidor local seleccionado y envía una desconexión individual por cada enlace RPT, incluidos los permanentes. Después vuelve a consultar hasta comprobar que no quedan enlaces y solo entonces solicita la nueva conexión con el nodo, modo, permanencia y timeout confirmados. No afecta a los demás servidores ni utiliza el filtro visual del monitor. Si una consulta o desconexión falla, o la lista no queda vacía en los 15 segundos posteriores a las desconexiones, no se inicia la nueva conexión y se muestra un error; los enlaces ya desconectados no se restauran automáticamente.

Las emisiones por intervalos comprueban COS y recepción RPT/IAX/SIP, exigen observaciones de silencio separadas por al menos cinco segundos y vuelven a consultar después de preparar el audio. La programación y el bloqueo se reservan juntos antes del envío para evitar reclamar dos veces la misma emisión. El principal se resuelve desde la configuración del monitor, con nodos locales de `rpt.conf` y AMI cuando no existe Allmon. Son comprobaciones por sondeo: no impiden que una estación empiece a hablar después de iniciada la reproducción.

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

Los errores de configuración no abortan el `postinstall` ni dejan el paquete pendiente de configurar en `dpkg`. Cada paso se ejecuta por separado: si falla, informa del error y muestra un aviso, detiene ese paso y continúa con los demás. Esto no garantiza que los servicios o funciones afectados estén operativos; corrige los avisos antes de usarlos.

La telemetría EchoLink configura `echolinkdefault = 1` en la plantilla `[node-main](!)` de `rpt.conf`, si existe, y en las secciones de nodo que corresponden a los `astnode` de `echolink.conf`. La plantilla aplica a los nodos que la heredan, salvo que tengan un valor propio; los nodos que coinciden con `astnode` se configuran explícitamente. No es necesario que exista una sección para un `astnode` de ejemplo como `1999` si está la plantilla. Sin plantilla, los nodos que no coincidan se omiten con un aviso; si no hay ninguna coincidencia, se conserva `rpt.conf`. Valores `astnode` inválidos, secciones duplicadas o plantillas ambiguas son errores y no modifican el archivo.

PJSIP usa una IPv4 privada de una interfaz activa: prefiere la interfaz de la ruta predeterminada y, si no es privada, la primera dirección RFC1918 encontrada. El instalador reemplaza `bind=0.0.0.0` de `[transport-udp]`, conserva binds específicos y respalda los cambios. Solo usa el comodín si no hay una IPv4 privada utilizable; antes de cargar PJSIP instala y verifica un bloqueo de UDP/5060 hacia IPv4 locales no privadas. Reutiliza firewalld activo o iptables; si no existe ninguno, instala iptables. Las reglas no modifican SSH, otros puertos ni políticas generales y se conservan tras reiniciar mediante firewalld permanente y `amerimon-pjsip-firewall.service`, requerido antes de Asterisk. Un fallo al aplicar la protección detiene el paso PJSIP sin cargar sus módulos ni abortar el postinstall; se informa del fallo y la instalación continúa. El paquete agrega `iproute2`, `net-tools` y la alternativa `iptables | firewalld`; desde fuentes, `install.sh` requiere `ip` y `netstat`. Si la IP pública pertenece a un router NAT y no al servidor, también debes bloquear el reenvío de UDP/5060 en ese router: el bind privado no impide por sí solo un port-forward.

El paquete `.deb` declara `libc-bin`, `libnss-mdns` y `avahi-daemon` como dependencias para resolver servidores `nombre.local` mediante mDNS sin depender de cgo. APT instala esas dependencias automáticamente. `avahi-utils` es opcional para diagnóstico; no es necesario para ejecutar Amerimon. Para instalar un `.deb` descargado y resolver sus dependencias, usa `sudo apt install ./amerimon_<version>_<arquitectura>.deb`, no solamente `dpkg -i`.

El servidor remoto debe publicar su nombre y la red debe permitir mDNS. Consulta [Usar hostname en la LAN](docs/hostname-lan.md). `install.sh` es una herramienta de instalación desde fuentes para desarrollo, no el mecanismo de distribución a otros equipos.

## Limpieza posterior a la distribución

Usa `make clean` para eliminar únicamente el frontend generado de `internal/web/dist`, conservando `.gitkeep` para `go:embed`. Al finalizar correctamente GoReleaser mediante `make release` (incluido en `make dist`), se llama automáticamente a ese mismo objetivo. Los binarios y paquetes de `dist/` ya contienen el frontend y no se modifican. Si la distribución falla, el frontend se conserva para diagnóstico. Si ejecutas GoReleaser directamente, la limpieza no es automática. Una compilación posterior vuelve a crear el frontend; puedes limpiarlo de nuevo con `make clean`. Para volver a ejecutar el binario desde fuentes con interfaz embebida, usa `make run` o ejecuta antes `npm run build`.

Los tests se conservan junto al código que prueban: los `_test.go` en su paquete Go y los tests de instalación junto a los scripts en `packaging/`. La limpieza no los elimina ni los regenera. El script de limpieza se verifica con `node --test scripts/clean-web-dist.test.cjs`.

## Primer acceso

Los botones de guardado de configuración y los diálogos de edición permanecen desactivados hasta que haya cambios respecto a los valores cargados. Restaurar los valores originales vuelve a desactivarlos; guardar correctamente o recargar establece una nueva referencia. Se conservan las validaciones, el bloqueo durante operaciones y los cambios pendientes cuando falla un guardado. Las acciones de consulta y conexión no requieren modificar una configuración.

### Usuarios IAX y extensiones PJSIP

En **Admin → Más opciones → Usuarios IAX / Extensiones PJSIP** se abren páginas de administración con listado, creación, edición y eliminación. Los formularios conservan los datos cuando falla una operación y muestran el diagnóstico del backend; **Recargar** permite reintentar la consulta.

Al guardar una sección IAX cuyo nombre no coincide con el formato reconocido de indicativo de radioaficionado, la aplicación pide confirmación porque podría pertenecer a la configuración del sistema. Esta advertencia admite **No volver a mostrar este tipo de aviso** por navegador y no se aplica a las extensiones PJSIP, cuyos identificadores no tienen que ser indicativos.

IAX utiliza `iax.conf` junto al archivo configurado en `ami.rpt_config_path`; PJSIP utiliza `ami.pjsip_config_path`. El usuario del servicio Amerimon necesita lectura y escritura sobre estos archivos y su directorio para crear el respaldo `.amerimon.bak` y reemplazar el contenido de forma atómica, conservando propietario y permisos. Las recargas se envían por AMI (`iax2 reload` / `module reload res_pjsip.so`), con las credenciales ya configuradas, no ejecutando Asterisk como root. PJSIP recarga el módulo al crear, editar o eliminar extensiones. Si falla la recarga después de guardar, se informa explícitamente: revisa AMI y los registros de Asterisk antes de reintentar.

IAX oculta y protege las secciones `radio`, `allstar-sys` y `allstar-public`. Si existe `[iaxclient](!)`, los usuarios nuevos se guardan en el archivo definido por un único `#tryinclude` dentro de esa misma stanza y heredan de `iaxclient`. Las rutas relativas se resuelven desde el directorio de `iax.conf`; las absolutas se respetan. No se utilizan includes de otras stanzas ni comentarios para elegir el destino. Si falta el include o el destino es ambiguo, se informa un error sin modificar archivos. Sin la plantilla, los usuarios nuevos se siguen creando en `iax.conf`; los existentes no se trasladan.

Para eliminar un usuario IAX, abre su edición y pulsa **Eliminar**; se solicita confirmación independientemente del formato del nombre. La sección heredada `iaxrpt` permanece visible pero no se puede eliminar.

PJSIP agrupa cada endpoint con sus referencias `auth` y `aors`, aunque los tres objetos tengan nombres distintos, y conserva opciones no expuestas en los formularios. Para editar o eliminar se requiere un único objeto auth `userpass` y un único AOR, exclusivos del endpoint y presentes en el archivo principal. Las plantillas, los objetos compartidos y las configuraciones ambiguas se rechazan sin modificarlos; los archivos incluidos mediante `#include` no se recorren ni se editan, y se bloquean las operaciones sobre secciones que contienen estas directivas. Antes de trabajar con una configuración heredada o distribuida, revísala manualmente.

Abre `http(s)://<host>/amerimon/` con el proxy configurado, o `http://<IP-del-servidor>:8082/` si accedes directamente. Pulsa **Admin** y define el usuario y contraseña iniciales. El nombre de usuario debe tener entre 3 y 32 caracteres; la contraseña debe tener al menos 8. La contraseña se guarda como hash.

Las advertencias y confirmaciones permiten marcar **No volver a mostrar este tipo de aviso**, por tipo y para este navegador, incluso tras recargar o cerrar sesión. En las confirmaciones, la preferencia solo se guarda al confirmar, no al cancelar; las siguientes acciones de ese tipo se ejecutan sin otra confirmación. Los errores y los diagnósticos de operaciones fallidas siempre se muestran. Temporalmente, **Admin → Personalizar cabecera → Restablecer advertencias y confirmaciones** recupera todos los avisos sin cambiar la cabecera ni la sesión. Consulta los detalles en los manuales técnicos [español](public/assets/docs/technical-operations-manual.es.md) e [inglés](public/assets/docs/technical-operations-manual.en.md).

### Ubicación del clima

Cualquier visitante puede pulsar **Cambiar ubicación**, junto a la localidad consultada, para **Obtener mi ubicación**, volver a **Usar configuración** (solo si el administrador guardó una ubicación) o escribir una **Ubicación**. El clima se carga automáticamente al recibir los ajustes de cabecera, incluso sin ubicación configurada, y se actualiza al cambiar de idioma y cada cinco minutos; el botón de recarga junto al reporte permite actualizarlo manualmente. La selección solo afecta al clima de la página actual: no guarda ni modifica la configuración del administrador y se descarta al recargar. La geolocalización requiere HTTPS (o localhost) y autorización del navegador; si el permiso fue denegado previamente, hay que habilitarlo en los permisos del sitio.

### Tablas de conexiones AllStarLink

Al pulsar el número de servidor en la cabecera de una tabla se selecciona ese nodo en el formulario de conexión. Cada tabla permite contraer o desplegar su contenido sin ocultar la cabecera. El botón **Contraer todas las tablas / Desplegar todas las tablas** aplica la acción a todos los servidores, incluso si hay un filtro activo, conservando siempre sus cabeceras.

Al ejecutar una conexión se despliega automáticamente la tabla del servidor de destino. Al buscar conexiones se despliegan los servidores con resultados dentro del filtro seleccionado, incluso si los resultados llegan después de escribir la búsqueda. Las tablas sin coincidencias conservan su estado; borrar la búsqueda no las despliega.

Al navegar a otra pantalla y volver, AllStarLink conserva en memoria las tablas contraídas, el filtro de servidor, la búsqueda, el campo y sentido de ordenación, el intervalo de actualización y si el panel administrativo estaba abierto. Estos ajustes se restablecen al recargar la aplicación. Las conexiones se consultan de nuevo; si el servidor filtrado ya no existe, se muestran todos. No se restauran diálogos ni se ejecutan operaciones al volver.

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
