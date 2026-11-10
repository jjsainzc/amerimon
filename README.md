# Amerimon

Interfaz web para consultar AllStarLink, administrar nodos Allmon y escuchar el nodo principal de Asterisk.

## Instalación

Paquetes Debian disponibles para `arm64` y `amd64`, en ese repositorio ya están compilados y listos para ser instalados:

```bash
curl -fsSL https://raw.githubusercontent.com/jjsainzc/repo-deb/main/hc1lnm.gpg \
  | sudo gpg --dearmor --yes -o /usr/share/keyrings/hc1lnm-archive-keyring.gpg

echo "deb [signed-by=/usr/share/keyrings/hc1lnm-archive-keyring.gpg arch=$(dpkg --print-architecture)] https://jjsainzc.github.io/repo-deb/ ./" \
  | sudo tee /etc/apt/sources.list.d/amerinet.list

sudo apt update
sudo apt install amerimon
```

> **NOTA:** Si lo va a usar en Raspberries o Thin Clients (ej. Dell Wyse), recomiendo use la instalación previamente descrita.
> Si usted es desarrollador con conocimientos de Node.js/Angular/Go Language entonces este repositorio le puede servir.

### Configuración aplicada durante la instalación

El instalador habilita `amerimon.service` y, si detecta Apache o Nginx, intenta configurar el proxy para `/amerimon`. Sin servidor web, abre `http://<IP-del-servidor>:8082/`.

Los errores de configuración no abortan el `postinstall` ni dejan el paquete pendiente de configurar en `dpkg`. Cada paso se ejecuta por separado: si falla, informa del error y muestra un aviso, detiene ese paso y continúa con los demás. Esto no garantiza que los servicios o funciones afectados estén operativos; corrige los avisos antes de usarlos.

Las credenciales, los directorios, la clave de terminal, cada operación de systemd y el proxy web son pasos independientes. Un fallo al reiniciar Amerimon no omite el temporizador astdb ni el proxy. `install.sh` usa esta misma configuración aislada y muestra un aviso, sin abortar, si el estado final del servicio no es activo. Los fallos de compilación o de instalación de archivos necesarios sí detienen la instalación.

La telemetría EchoLink configura `echolinkdefault = 1` en la plantilla `[node-main](!)` de `rpt.conf`, si existe, y en las secciones de nodo que corresponden a los `astnode` de `echolink.conf`. La plantilla aplica a los nodos que la heredan, salvo que tengan un valor propio; los nodos que coinciden con `astnode` se configuran explícitamente. No es necesario que exista una sección para un `astnode` de ejemplo como `1999` si está la plantilla. Sin plantilla, los nodos que no coincidan se omiten con un aviso; si no hay ninguna coincidencia, se conserva `rpt.conf`. Valores `astnode` inválidos, secciones duplicadas o plantillas ambiguas son errores y no modifican el archivo.

PJSIP usa una IPv4 privada de una interfaz activa: prefiere la interfaz de la ruta predeterminada y, si no es privada, la primera dirección RFC1918 encontrada. El instalador reemplaza `bind=0.0.0.0` de `[transport-udp]`, conserva binds específicos y respalda los cambios. Solo usa el comodín si no hay una IPv4 privada utilizable; antes de cargar PJSIP instala y verifica un bloqueo de UDP/5060 hacia IPv4 locales no privadas. Reutiliza firewalld activo o iptables; si no existe ninguno, instala iptables. Las reglas no modifican SSH, otros puertos ni políticas generales y se conservan tras reiniciar mediante firewalld permanente y `amerimon-pjsip-firewall.service`, requerido antes de Asterisk. Un fallo al aplicar la protección detiene el paso PJSIP sin cargar sus módulos ni abortar el postinstall; se informa del fallo y la instalación continúa. El paquete agrega `iproute2`, `net-tools` y la alternativa `iptables | firewalld`; desde fuentes, `install.sh` requiere `ip` y `netstat`. Si la IP pública pertenece a un router NAT y no al servidor, también debes bloquear el reenvío de UDP/5060 en ese router: el bind privado no impide por sí solo un port-forward.

El paquete `.deb` declara `libc-bin`, `libnss-mdns` y `avahi-daemon` como dependencias para resolver servidores `nombre.local` mediante mDNS sin depender de cgo. APT instala esas dependencias automáticamente. `avahi-utils` es opcional para diagnóstico; no es necesario para ejecutar Amerimon. Para instalar un `.deb` descargado y resolver sus dependencias, usa `sudo apt install ./amerimon_<version>_<arquitectura>.deb`, no solamente `dpkg -i`.

El servidor remoto debe publicar su nombre y la red debe permitir mDNS. Consulta [Usar hostname en la LAN](docs/manuals/hostname-lan.md). `install.sh` es una herramienta de instalación desde fuentes para desarrollo, no el mecanismo de distribución a otros equipos.

## Administración

### Acceso inicial

Abre `http(s)://<host>/amerimon/` con el proxy configurado, o `http://<IP-del-servidor>:8082/` si accedes directamente. Pulsa **Admin** y define el usuario y contraseña iniciales. El nombre de usuario debe tener entre 3 y 32 caracteres; la contraseña debe tener al menos 8. La contraseña se guarda como hash.

### Pantalla Nextion

**Administración → Pantalla Nextion** permite configurar el dispositivo serie, la velocidad, la interfaz de red, el nodo favorito y el envío de etiquetas internacionalizadas. El firmware es único para todos los idiomas: los textos de botones y títulos se envían por serie según el idioma elegido en la cabecera de la web; si usas un diseño HMI propio puedes desactivar el envío de etiquetas para conservar tus textos y recibir solo los valores. Las ocho filas de enlaces muestran el número para IAX, el indicativo para IAXRpt y el indicativo disponible o número para EchoLink, sin prefijos; SIP conserva `SIP` seguido del caller ID o endpoint. Pulsar una fila mantiene la asociación con la conexión real aunque el texto mostrado sea un indicativo.

Durante la actividad, `t0.txt` envía el indicativo o número del origen del audio junto a su descripción disponible en AstDB o el catálogo privado, por ejemplo `700001 - Remote station`, sin prefijos ni textos de evento. Si no hay descripción, muestra solo el identificador; varias fuentes activas se separan por comas. Las filas de enlaces no incorporan la descripción. Para COS local o TX sin recepción usa el número y la descripción disponible del nodo local. SIP sigue mostrando solo su caller ID o endpoint. Verde indica recepción y tiene prioridad si Asterisk retransmite simultáneamente el audio; rojo indica TX sin recepción; gris indica ausencia de actividad y el texto queda vacío. Los mensajes temporales del sistema y los avisos de desconexión siguen teniendo prioridad. Consulta el [manual de la pantalla Nextion](docs/manuals/nextion.md) para componentes, colores y eventos.

Al agregar o editar favoritos, la búsqueda en AstDB se ejecuta con el botón de lupa o con Enter, no mientras se escribe. Si hay más de 50 coincidencias, se muestran solo las primeras 50 y un aviso para precisar la búsqueda, evitando renderizar listas demasiado grandes.

### Herramientas generales

Los campos de IP, nodo e indicativo y sus valores en tablas y vistas incluyen un icono para copiar el valor actual al portapapeles, sin enviar formularios ni abrir los enlaces. No se incluyen contraseñas ni secretos AMI. En nodos privados se copia la línea completa del editor. El control confirma «Copiado» o muestra un error si el navegador bloquea la operación. En HTTP sin API de portapapeles se intenta la copia compatible del navegador; si tampoco está permitida, se indica copiar manualmente.

### Nodos privados

**Administración → Nodos privados** reúne el catálogo `privatenodes.txt` y la configuración de nodos de cuatro dígitos en `rpt.conf` (ruta `ami.rpt_config_path`). Son editores independientes: guardar o borrar una línea del catálogo no crea ni elimina una sección de Asterisk. El listado de `rpt.conf` permite crear, editar en una tarjeta emergente y eliminar con confirmación la sección del nodo y su entrada en `[nodes]`.

Cada fila del listado de `rpt.conf` tiene una casilla **Mostrar como servidor en AllStarLink**, desactivada por defecto. Cambia las casillas y pulsa el icono de disquete (**Guardar cambios**), como en el listado Allmon. Los iconos de edición y papelera permiten editar y eliminar; todos muestran ayuda y tienen etiquetas accesibles traducidas. Los nodos marcados crean su tabla y aparecen en el selector de servidores locales, usando las credenciales AMI globales; siguen siendo privados y no se convierten en el principal. La preferencia persiste como comentario `; amerimon_show_as_server = yes` en la sección del nodo, sin agregar una opción de Asterisk. Al desmarcar y guardar se retira ese comentario; no se desconectan sus enlaces.

Los nodos nuevos heredan `[node-main]` y empiezan con los valores del ejemplo 1998: `rxchannel=USRP/127.0.0.1:34001:32001`, `duplex=0`, `hangtime=0`, `althangtime=0`, `holdofftelem=1`, `telemdefault=0`, `telemdynamic=0`, `linktolink=no`, `nounkeyct=1` y `totime=180000`. El número debe introducirse; el canal y los parámetros son editables. La ayuda de `rxchannel` está dentro del editor. No se permiten números ni canales repetidos; se comprueban también coincidencias del par de puertos USRP y canales de nodos públicos definidos directamente en el archivo.

El combo recoge los parámetros activos de `[node-main]` que todavía no tienen sobrescritura en el nodo, precarga el valor de la plantilla y muestra sus comentarios anteriores e internos como documentación. **Escribir otro parámetro** permite nombres personalizados. Las sobrescrituras se guardan únicamente en la sección del nodo, sin modificar la plantilla ni eliminar parámetros omitidos del formulario. La interfaz está traducida al español y al inglés; los comentarios de configuración se muestran en su idioma original. Consulta las instrucciones completas en los [manuales técnicos en español](public/assets/docs/technical-operations-manual.es.md) y [en inglés](public/assets/docs/technical-operations-manual.en.md).

Al guardar nodos privados se rechazan números que coincidan con IDs de secciones PJSIP, incluidos endpoints, auth, AOR y plantillas. Se usa `ami.pjsip_config_path` o `pjsip.conf` junto a `rpt.conf`, recorriendo `#include` y `#tryinclude` (también con comodines). Si la configuración no puede comprobarse por errores de lectura, includes obligatorios ausentes, ciclos o directivas no admitidas como `#exec`, no se guarda ni se recarga Asterisk. Un `#tryinclude` sin coincidencias es válido.

Antes de cambiar `rpt.conf`, Amerimon guarda una copia `.amerimon.bak` y conserva permisos, propietario y grupo del destino, incluidos los enlaces simbólicos. Al guardar o eliminar solicita `module reload app_rpt.so`, exige confirmación de recarga y consulta `module show like app_rpt.so`. Solo indica éxito si el módulo aparece cargado. Si falla AMI o la verificación, el archivo puede haber quedado guardado y se informa el error; no se reinicia Asterisk ni se revierte automáticamente. Esta comprobación no sustituye una prueba de audio o conexión de cada nodo.

### Reconexión automática

**Administración → Reconexión automática** mantiene una lista persistente de destinos AllStarLink, privados o EchoLink. Cada entrada tiene un **Nodo fuente**, seleccionado en el combo de servidores de AllStarLink: locales, privados marcados como servidores o remotos de Allmon. Usa las credenciales AMI de esa fuente y permite el mismo destino desde fuentes diferentes, sin duplicar parejas. Las entradas anteriores conservan **Principal actual**; las nuevas preseleccionan su número cuando está disponible. Si la fuente desaparece, se informa el error sin redirigir al principal. Para EchoLink introduce el identificador de app_rpt: `3` seguido del número EchoLink (por ejemplo, `3123456` para `123456`). Cada entrada tiene activación individual, frecuencia entera de 1 a 86400 segundos y modo transceptor, monitor o monitor local, temporal o permanente. Empieza desactivada con frecuencia de 60 segundos. El temporizador corre en el servidor, sin navegador ni cron externo: consulta `XStat` y solo solicita `ilink` si el destino exacto está ausente en esa fuente, respetando enlaces existentes de cualquier modo y las comprobaciones de topología y reservas pendientes. Desactivar o eliminar no desconecta. La tarjeta destaca fuente y destino, agrupa acciones por iconos y separa modo, frecuencia, estado y fechas; «solicitada» no significa «confirmada». Las migraciones `0016_node_reconnections.sql` y `0017_node_reconnection_sources.sql` conservan entradas, ajustes y fechas del cliente y no cambian YAML, `rpt.conf`, Allmon ni sus conexiones permanentes.

En reconexión automática, la tarjeta muestra **fuente ← destino**: el destino es el nodo que se conecta al fuente, no una indicación del sentido del audio. Al desactivar y guardar una entrada, el temporizador no comprueba ese enlace ni intenta reconectarlo; la tarjeta indica **Temporizador detenido** y oculta la próxima comprobación y los diagnósticos anteriores. La lista no se recarga automáticamente ni muestra etiquetas «Cargando»: se lee al entrar, después de guardar o eliminar y mediante recarga manual. Las fechas y resultados visibles corresponden a esa lectura, no a una actualización en vivo. El temporizador del servidor es independiente de la pantalla.

### Gestión de audios

**Administración → Gestión de audios** administra una biblioteca persistente de anuncios para AllStarLink por IAX/IAXRPT. Acepta WAV, MP3, FLAC, OGG/Opus, M4A/MP4, AAC, WMA, AIFF, WebM, MKA, AMR y APE; convierte en el servidor a WAV PCM mono de 16 bits y 8000 Hz mediante FFmpeg. Entrada y resultado tienen un límite de 25 MiB. El instalador local requiere `ffmpeg` y el paquete Debian lo incluye como dependencia. Los audios y sus activaciones permanecen en el servidor aunque el administrador cierre sesión o la web; solo se eliminan por la acción explícita **Eliminar**. Las emisiones por intervalo pueden ser varias, pero se serializan para no superponerse. La emisión programada solo se intenta con silencio total confirmado en todos los nodos monitorizados.

### Grabaciones

La tarjeta **Grabaciones del nodo principal** captura la escucha del nodo y sus estaciones conectadas en el servidor: iniciar, pausa manual, continuar y detener/guardar. Omite silencios automáticamente y reanuda al detectar audio sin anular una pausa manual. Permite escuchar solo en el navegador, borrar con confirmación o añadir una copia independiente a la biblioteca para emitirla o programarla después. Reutiliza EAGI/Listen Live; no modifica configuraciones de Asterisk. La captura sigue sin la web y se limita a 25 MiB por archivo; tras una interrupción del servicio se recupera el WAV, pero no se reanuda la captura sola.

La pantalla Angular es un módulo reutilizable en `src/app/features/audio-management`. Para integrarlo en otra app se proporciona la URL del API, un traductor para las claves `AUDIO_*` y una ruta protegida por la autorización de administrador del proyecto. Consulta la [guía del módulo](docs/manuals/audio-management.es.md) y el [manual técnico](public/assets/docs/technical-operations-manual.es.md).

Los botones de audio usan iconos con descripciones y etiquetas accesibles ES/EN. En la biblioteca, los auriculares permiten escuchar localmente sin emitir ni cambiar la programación; el icono de emisión reproduce en Asterisk los audios activados de emisión única. Grabación y escucha local incluyen medidores RMS en dBFS. Durante las solicitudes se bloquean las otras acciones y el proceso aparece solo dentro del botón correspondiente, incluido el porcentaje de subida.

La configuración de fragmento recuerda el inicio y el final por audio, tanto para emisiones manuales como para intervalos, sin modificar el archivo original. Una sola línea de tiempo muestra ambos extremos y resalta la selección; también permite escribir los segundos de inicio y final. **Escuchar selección** incorpora un reproductor con medidor dentro del cuadro, sin guardar ni transmitir a Asterisk. El disquete guarda los límites sin emitir y la tarjeta muestra **Fragmento seleccionado**, con inicio, final y duración. **Restablecer audio completo** y guardar elimina la selección recordada. Los auriculares de la tarjeta y la descarga siguen ofreciendo el audio completo. El servidor genera un WAV temporal para cada emisión del fragmento. El diálogo, validaciones y manuales están disponibles en español e inglés.

Las emisiones por intervalos comprueban COS y recepción RPT/IAX/SIP, exigen observaciones de silencio separadas por al menos cinco segundos y vuelven a consultar después de preparar el audio. La programación y el bloqueo se reservan juntos antes del envío para evitar reclamar dos veces la misma emisión. El principal se resuelve desde la configuración del monitor, con nodos locales de `rpt.conf` y AMI cuando no existe Allmon. Son comprobaciones por sondeo: no impiden que una estación empiece a hablar después de iniciada la reproducción.

### Silenciar estaciones del servidor principal

El administrador dispone de una bocina junto al botón de desconexión de cada enlace RPT del nodo principal. Permite bloquear el audio que esa estación envía al servidor mediante AMI `MuteAudio`, `Direction: in`, manteniendo su conexión y su escucha. No libera necesariamente un PTT bloqueado ni controla las conexiones de la estación con otros servidores.

El postinstall incorpora `res_mutestream.so` a `/etc/asterisk/modules.conf` si está instalado, conservando una copia de seguridad y eliminando directivas `noload` contradictorias. No reinicia Asterisk. El usuario AMI necesita permiso para `MuteAudio` (clase `system`) y las consultas de canales. No se habilita el módulo en servidores remotos.

El botón queda deshabilitado si falta soporte o no puede asociarse de forma inequívoca el enlace con su canal. Una bocina tachada indica el último mute confirmado por Asterisk; el borde discontinuo indica estado desconocido, no audio necesariamente habilitado. Los estados se guardan en memoria del backend por identificador de conexión, se pierden al reiniciar Amerimon y no detectan cambios hechos por herramientas externas. Una reconexión requiere volver a aplicar el mute. La aplicación nunca silencia automáticamente ni cambia el icono a un estado confirmado antes de recibir la aceptación AMI.

### Comportamiento de los formularios

Los botones de guardado de configuración y los diálogos de edición permanecen desactivados hasta que haya cambios respecto a los valores cargados. Restaurar los valores originales vuelve a desactivarlos; guardar correctamente o recargar establece una nueva referencia. Se conservan las validaciones, el bloqueo durante operaciones y los cambios pendientes cuando falla un guardado. Las acciones de consulta y conexión no requieren modificar una configuración.

### Usuarios IAX y extensiones PJSIP

En **Admin → Más opciones → Usuarios IAX / Extensiones PJSIP** se abren páginas de administración con listado, creación, edición y eliminación. Los formularios conservan los datos cuando falla una operación y muestran el diagnóstico del backend; **Recargar** permite reintentar la consulta.

Al guardar una sección IAX cuyo nombre no coincide con el formato reconocido de indicativo de radioaficionado, la aplicación pide confirmación porque podría pertenecer a la configuración del sistema. Esta advertencia admite **No volver a mostrar este tipo de aviso** por navegador y no se aplica a las extensiones PJSIP, cuyos identificadores no tienen que ser indicativos.

IAX utiliza `iax.conf` junto al archivo configurado en `ami.rpt_config_path`; PJSIP utiliza `ami.pjsip_config_path`. El usuario del servicio Amerimon necesita lectura y escritura sobre estos archivos y su directorio para crear el respaldo `.amerimon.bak` y reemplazar el contenido de forma atómica, conservando propietario y permisos. Las recargas se envían por AMI (`iax2 reload` / `module reload res_pjsip.so`), con las credenciales ya configuradas, no ejecutando Asterisk como root. PJSIP recarga el módulo al crear, editar o eliminar extensiones. Si falla la recarga después de guardar, se informa explícitamente: revisa AMI y los registros de Asterisk antes de reintentar.

IAX oculta y protege las secciones `radio`, `allstar-sys` y `allstar-public`. Si existe `[iaxclient](!)`, los usuarios nuevos se guardan en el archivo definido por un único `#tryinclude` dentro de esa misma stanza y heredan de `iaxclient`. Las rutas relativas se resuelven desde el directorio de `iax.conf`; las absolutas se respetan. No se utilizan includes de otras stanzas ni comentarios para elegir el destino. Si falta el include o el destino es ambiguo, se informa un error sin modificar archivos. Sin la plantilla, los usuarios nuevos se siguen creando en `iax.conf`; los existentes no se trasladan.

Para eliminar un usuario IAX, abre su edición y pulsa **Eliminar**; se solicita confirmación independientemente del formato del nombre. La sección heredada `iaxrpt` permanece visible pero no se puede eliminar.

PJSIP agrupa cada endpoint con sus referencias `auth` y `aors`, aunque los tres objetos tengan nombres distintos, y conserva opciones no expuestas en los formularios. Para editar o eliminar se requiere un único objeto auth `userpass` y un único AOR, exclusivos del endpoint y presentes en el archivo principal. Las plantillas, los objetos compartidos y las configuraciones ambiguas se rechazan sin modificarlos; los archivos incluidos mediante `#include` no se recorren ni se editan, y se bloquean las operaciones sobre secciones que contienen estas directivas. Antes de trabajar con una configuración heredada o distribuida, revísala manualmente.

### Avisos y confirmaciones

Las advertencias y confirmaciones permiten marcar **No volver a mostrar este tipo de aviso**, por tipo y para este navegador, incluso tras recargar o cerrar sesión. En las confirmaciones, la preferencia solo se guarda al confirmar, no al cancelar; las siguientes acciones de ese tipo se ejecutan sin otra confirmación. Los errores y los diagnósticos de operaciones fallidas siempre se muestran. Temporalmente, **Admin → Personalizar cabecera → Restablecer advertencias y confirmaciones** recupera todos los avisos sin cambiar la cabecera ni la sesión. Consulta los detalles en los manuales técnicos [español](public/assets/docs/technical-operations-manual.es.md) e [inglés](public/assets/docs/technical-operations-manual.en.md).


## Uso de la aplicación

### Monitor de conexiones

El monitor consulta cada servidor del grupo de forma independiente. Si un servidor no responde, reintenta su conexión en segundo plano cada 5 segundos sin bloquear los demás, incluso con la actualización automática desactivada. Los fallos de consulta de nodos, incluido el principal, no muestran mensajes de error: su grupo indica únicamente que está reintentando en segundo plano. Los errores al obtener la configuración del monitor y los de operaciones solicitadas por el usuario siguen mostrándose.

La comprobación de topología utiliza los nodos que responden con una lista interpretable y las conexiones pendientes conocidas. Si otros nodos no responden o no se pueden consultar, no bloquea la conexión ni muestra un error de validación de topología; el diagnóstico queda en el log del servidor. Los autoenlaces y las rutas que sí se detectan siguen bloqueados. Con información incompleta no se puede garantizar la ausencia de bucles.

### Conectar nodos

El selector de servidor local permanece sincronizado con el nodo utilizado en las conexiones y operaciones Asterisk, incluso al cargar o actualizar la lista de servidores. La confirmación conserva los parámetros mostrados: cambios posteriores del formulario no alteran la solicitud confirmada. Si el servidor deja de estar disponible en la lista antes de confirmar, la operación se bloquea con un aviso.

El panel **Conectar nodo remoto** conserva el servidor local, nodo remoto, modo, permanencia, desconexión previa y timeout al navegar a otra pantalla y regresar, aunque todavía no se haya enviado el formulario. Estos valores se mantienen en memoria hasta recargar la aplicación. Al regresar se comprueba que el servidor recordado siga disponible; si ya no existe, se selecciona el nodo principal disponible o se deja el selector vacío. Recuperar los valores no ejecuta ninguna operación.

Al conectar un nodo que ya tiene un enlace directo en el servidor seleccionado, Amerimon compara el modo actual y la permanencia del último comando de conexión registrado. Si coinciden con lo solicitado, avisa sin enviar comandos. Si difieren, desconecta únicamente ese enlace (incluidos los permanentes), espera a que XStat confirme su desaparición y vuelve a comprobar la protección contra bucles antes de conectarlo con la nueva modalidad. Si falla la desconexión o no se confirma en 15 segundos, no reconecta. XStat no informa la permanencia: para enlaces sin un registro de conexión compatible, por ejemplo creados fuera de Amerimon, la confirmación advierte que se reconectará para aplicar los valores solicitados. Los cambios externos de permanencia no son detectables por XStat.

En **Conectar nodo remoto**, la casilla **Desconectar todos antes de conectar** está desmarcada por defecto. Si se activa, el componente consulta la lista actualizada del servidor local seleccionado y envía una desconexión individual por cada enlace RPT, incluidos los permanentes. Después vuelve a consultar hasta comprobar que no quedan enlaces y solo entonces solicita la nueva conexión con el nodo, modo, permanencia y timeout confirmados. No afecta a los demás servidores ni utiliza el filtro visual del monitor. Si una consulta o desconexión falla, o la lista no queda vacía en los 15 segundos posteriores a las desconexiones, no se inicia la nueva conexión y se muestra un error; los enlaces ya desconectados no se restauran automáticamente.

### Ubicación del clima

Cualquier visitante puede pulsar **Cambiar ubicación**, junto a la localidad consultada, para **Obtener mi ubicación**, volver a **Usar configuración** (solo si el administrador guardó una ubicación) o escribir una **Ubicación**. El clima se carga automáticamente al recibir los ajustes de cabecera, incluso sin ubicación configurada, y se actualiza al cambiar de idioma y cada cinco minutos; el botón de recarga junto al reporte permite actualizarlo manualmente. La selección solo afecta al clima de la página actual: no guarda ni modifica la configuración del administrador y se descarta al recargar. La geolocalización requiere HTTPS (o localhost) y autorización del navegador; si el permiso fue denegado previamente, hay que habilitarlo en los permisos del sitio.

### Tablas de conexiones AllStarLink

Al pulsar el número de servidor en la cabecera de una tabla se selecciona ese nodo en el formulario de conexión. Cada tabla permite contraer o desplegar su contenido sin ocultar la cabecera. El botón **Contraer todas las tablas / Desplegar todas las tablas** aplica la acción a todos los servidores, incluso si hay un filtro activo, conservando siempre sus cabeceras.

Al ejecutar una conexión se despliega automáticamente la tabla del servidor de destino. Al buscar conexiones se despliegan los servidores con resultados dentro del filtro seleccionado, incluso si los resultados llegan después de escribir la búsqueda. Las tablas sin coincidencias conservan su estado; borrar la búsqueda no las despliega.

Al navegar a otra pantalla y volver, AllStarLink conserva en memoria las tablas contraídas, el filtro de servidor, la búsqueda, el campo y sentido de ordenación, el intervalo de actualización y si el panel administrativo estaba abierto. Estos ajustes se restablecen al recargar la aplicación. Las conexiones se consultan de nuevo; si el servidor filtrado ya no existe, se muestran todos. No se restauran diálogos ni se ejecutan operaciones al volver.

## Operación y soporte

### Restablecer contraseña de administrador

Si se olvidó la contraseña, ejecuta en el servidor:

```bash
sudo amerimon-reset-password
```

El comando borra únicamente la cuenta de administrador para que puedas definirla de nuevo desde **Admin**. No borra nodos Allmon, favoritos, registros ni el resto de la base de datos. Después, vuelve a abrir la web y define el usuario y la contraseña.

### Diagnosticar Listen Live

Para consultar canales o procesos EAGI residuales sin interrumpir escuchas:

```bash
sudo limpiar_canales_listen_live.sh --check
```

La opción `--clean` solicita hangup y termina procesos EAGI huérfanos, pero interrumpe todas las escuchas activas y pide confirmación escribiendo `LIMPIAR`.

### Servicio y actualización

```bash
sudo systemctl status amerimon --no-pager
sudo apt update
apt list --upgradable
sudo apt upgrade
```

## Desarrollo

Esta sección es para quienes modifican o publican el proyecto. Para instalar o actualizar un servidor, utiliza APT como se indica al inicio; no necesitas estas herramientas.

### Ejecución local sin depurador

Necesitas Go compatible con `go.mod`, las dependencias Angular (`npm ci`) y acceso a una configuración Asterisk de pruebas. Para convertir audios instala `ffmpeg`; para resolver nombres `.local`, prepara `libc-bin`, `libnss-mdns` y `avahi-daemon`. `install.sh` comprueba `getent`, pero no instala ni configura Avahi/NSS.

Antes de iniciar, detén el servicio instalado para liberar `8082` y `8086`:

```bash
sudo systemctl stop amerimon
make run
```

`make run` compila Angular, lo incrusta y ejecuta Go en `127.0.0.1:8082`, con SQLite `/tmp/amerimon-dev.db`, JWT `dev-secret`, `packaging/amerimon.yaml` y las credenciales de `/etc/asterisk/manager.conf`. Termina con `Ctrl+C`. Para recarga separada, utiliza dos terminales: `make dev-backend` y `npm start`; Angular escucha en `localhost:4200` y reenvía `/mon` al backend.

**La base de desarrollo no aísla Asterisk:** las acciones administrativas pueden modificar los archivos y nodos reales indicados en el YAML. No ejecutes dos backends ni una instancia local junto al servicio instalado.

Para instalar el código actual en el servidor, ejecuta `./install.sh`. Recompila frontend/backend, actualiza `/usr/bin/amerimon` e instala EAGI, la migración del dialplan y los auxiliares del servicio. Ejecuta `packaging/postinstall.sh` para migrar Listen Live con respaldo, habilitar módulos disponibles, recargar el dialplan, aplicar el proxy y el temporizador AstDB y reiniciar Amerimon. Conserva configuración, credenciales y Allmon existentes. Este paso modifica el sistema; no lo uses como sustituto de APT para distribuir la aplicación.

Al terminar una ejecución local, restaura el servicio si lo habías detenido:

```bash
sudo systemctl start amerimon
sudo systemctl status amerimon --no-pager
```

### Depuración en VS Code

Consulta la [guía paso a paso de depuración de Angular y Go](docs/manuals/depuracion-vscode.md).

1. Instala las dependencias con `npm ci` si aún no están instaladas y asegúrate de tener Go (la versión indicada en `go.mod`).
2. Instala la extensión recomendada **Go** (`golang.go`) en el entorno donde abres el proyecto (también en SSH/WSL, si corresponde). Si VS Code solicita instalar **Delve**, acepta.
3. Libera los puertos `4200`, `8082` y `8086`: detén otras ejecuciones de desarrollo. Si está activo el servicio instalado, usa `sudo systemctl stop amerimon`.
4. En **Ejecutar y depurar**, selecciona **Angular + Go** y pulsa **F5**.

El navegador abre `http://localhost:4200/`. Angular reenvía las peticiones `/mon` al backend Go en `127.0.0.1:8082`. Puedes poner breakpoints tanto en TypeScript como en Go. La configuración **Backend Go** permite depurar solo el backend; **ng serve** solo inicia Angular.

La depuración usa `packaging/amerimon.yaml`, la base SQLite `/tmp/amerimon-dev.db` y un secreto JWT exclusivo de desarrollo (`dev-secret`). No uses este secreto en producción. Las rutas de Asterisk y Allmon siguen siendo las del YAML: las acciones administrativas pueden modificar archivos y nodos reales.

Antes de iniciar Go, una tarea prepara las credenciales AMI: usa `AMI_USER` y `AMI_SECRET` del entorno de VS Code o, si faltan, busca una cuenta con contraseña en `/etc/asterisk/manager.conf`. Las guarda en `.vscode/ami.env`, excluido de Git y con permisos `0600`. Si no puede leer la configuración o encontrar una cuenta, la tarea falla con un mensaje explícito. No guardes credenciales en `launch.json`.

La tarea usa la primera cuenta con `secret` definido directamente, o la cuenta indicada por `AMI_USER`. Para cuentas definidas solo mediante plantillas o inclusiones, define ambas variables en el entorno de VS Code; en SSH/WSL debe ser el entorno remoto. No ejecutes VS Code como root para leer las credenciales.

Para inspeccionar la resolución AMI, coloca un breakpoint en `dialAMI` de `internal/config/asteriskami.go` y revisa `host`, `addresses` y `conn.RemoteAddr()`. `.local` se resuelve mediante `getent ahosts` y NSS/Avahi, incluso con `CGO_ENABLED=0`; los demás nombres usan el resolvedor Go.

**Angular + Go** ejecuta esa tarea automáticamente antes de resolver las configuraciones del depurador. Para iniciar **Backend Go** por separado, ejecuta primero **Tareas: Ejecutar tarea → Preparar AMI para depurar**.

**Detener** finaliza las sesiones de depuración de Angular y Go. La tarea `npm: start` puede seguir activa: termínala con **Tareas: Terminar tarea** antes de volver a iniciarla. Si detuviste el servicio instalado, restáuralo al terminar con `sudo systemctl start amerimon`.

Coloca los breakpoints en líneas ejecutables y realiza la acción correspondiente desde el navegador. Inspecciona **Variables**, **Inspección** y **Pila de llamadas**; abrir una página no ejecuta todos los controladores Go.

| Tecla | Acción |
| --- | --- |
| `F5` | Continuar |
| `F10` | Paso por procedimientos |
| `F11` | Entrar en una función |
| `Shift+F11` | Salir de una función |
| `Shift+F5` | Detener |

Para diagnosticar puertos ocupados:

```bash
ss -ltn '( sport = :4200 or sport = :8082 or sport = :8086 )'
```

Detén la ejecución anterior; no inicies `make dev-backend` junto al depurador. Si solo inicia Angular, selecciona **Angular + Go**, no **ng serve**. Si Go no inicia, comprueba la extensión Go y Delve en el entorno del proyecto. Si AMI falla, revisa la tarea, permisos y conectividad; después de cambiar credenciales, detén Go y vuelve a iniciar con F5.

Con ambos servicios activos, estas comprobaciones deben devolver `{"status":"ok"}`:

```bash
curl -fsS http://127.0.0.1:8082/mon/health
curl -fsS http://localhost:4200/mon/health
```

### Integración del módulo Angular de audio

El módulo standalone de `src/app/features/audio-management` puede reutilizarse en otra aplicación. El anfitrión proporciona autenticación, interceptores HTTP, autorización administrativa de ruta, navegación y traducciones. La guía completa se conserva fuera de la web en [español](docs/manuals/audio-management.es.md) e [inglés](docs/manuals/audio-management.en.md).

- Importa `AudioManagementComponent` y registra `provideAudioManagement(baseUrl)` en los proveedores de la ruta protegida. La base real de Amerimon es `/mon/audio`; `/api/audio` en los ejemplos de integración es ilustrativo.
- Proporciona `AUDIO_MANAGEMENT_TRANSLATE` o pasa un traductor a `provideAudioManagement`. El predeterminado devuelve claves; incluye traducciones `AUDIO_*`, `AUDIO_RECORD_*` y `AUDIO_LEVEL_*` para etiquetas, confirmaciones, éxito y errores.
- `audioLabel` es impura. Si el anfitrión usa signals, el traductor debe leer la revisión de idioma/diccionario; Amerimon lee `TranslationService.revision()` para actualizar etiquetas al cambiar idioma o cargar traducciones.
- `AUDIO_MANAGEMENT_MONITOR_URL` es opcional y de mismo origen; Amerimon usa `allstarlink`. El acceso directo permanece oculto sin proveedor. Las ventanas emergentes notifican bloqueos y no transfieren tokens.
- `AudioRecordingsComponent` puede importarse por separado con los mismos proveedores. Al copiar una grabación, `AudioManagementService.libraryChanged` actualiza la biblioteca. Borrar el original no elimina su copia.
- `AudioPlayerComponent` libera las URLs y el contexto Web Audio al destruirse y restablece el medidor al pausar/finalizar. `AudioLevelMeterComponent` se reutiliza con `dbfs` y `label`; mide señal, no volumen físico. Los niveles son transitorios y no requieren migración ni permiso de micrófono.
- Los estilos globales de campos de texto deben excluir `input[type=range]` para no ocultar la línea de tiempo. Mantén `aria-busy`, etiquetas accesibles y progreso dentro del botón iniciador. El sondeo de grabaciones no debe bloquear los controles de captura.
- Los errores incluyen `code: AUDIO_ERROR_*` y `message` de diagnóstico: muestra el código traducido, sin asumir que el mensaje del backend está localizado.

Las siguientes rutas son relativas a la base de audio y requieren las solicitudes autenticadas del anfitrión:

| Método y ruta | Contrato |
| --- | --- |
| `GET /tracks` | `{ status, data: AudioTrack[] }` |
| `POST /tracks` | Multiparte: `name`, `playback_mode`, `interval_minutes` para modo silencio y `file` |
| `PATCH /tracks/{id}` | Ajustes de pista y límites opcionales `start_ms`, `end_ms` |
| `DELETE /tracks/{id}` | Elimina pista, archivo y programación |
| `POST /tracks/{id}/play` | Sin cuerpo usa el segmento guardado; límites explícitos solo aplican a esa solicitud |
| `GET /tracks/{id}/file` | WAV autenticado completo para vista previa/descarga |
| `GET /recordings` | `{ status, data: AudioRecording[] }` |
| `POST /recordings` | `{ name }`; inicio confirmado tras recibir PCM del servidor |
| `PATCH /recordings/{id}` | `{ action: 'pause' \| 'resume' \| 'stop' }` |
| `GET /recordings/{id}/file` | WAV autenticado |
| `DELETE /recordings/{id}` | Elimina la grabación |
| `POST /recordings/{id}/library` | Copia independiente sin transmitir |

Los límites se envían juntos en milisegundos enteros, con inicio inclusivo y fin exclusivo: `0 <= start_ms < end_ms <= duration_ms`. `{ start_ms: 0, end_ms: 0 }` restaura el audio completo. La migración `0015` mantiene completas las pistas existentes. Guardar no transmite; transmitir guarda y luego emite. Cancelar descarta cambios y cambiar límites no reinicia el intervalo. La vista previa del diálogo utiliza `audioSegmentBlob` para cortar PCM normalizado en el navegador; nunca guarda ni transmite, y libera las URLs al cambiar límites o cerrar. Las vistas previas ordinarias y descargas siguen completas.

La captura usa EAGI y la mezcla `Rpt(node,Pm)`, no el micrófono ni el WebSocket del navegador; cerrar el componente detiene el sondeo, no la captura del servidor. El nivel de entrada usa RMS `level_dbfs` con piso de −60 dBFS, retención de 500 ms y caducidad de un segundo; se sondea cada 500 ms durante captura y cada dos segundos en reposo. Los medidores de reproducción local muestrean cada 50 ms.

### Pruebas

Los tests se conservan junto al código que prueban: los `_test.go` en su paquete Go y los tests de instalación junto a los scripts en `packaging/`. Las pruebas de nodos privados, nodo principal y mantenimiento usan archivos temporales o comandos/AMI simulados, sin modificar ni reiniciar un nodo real.

La pantalla EchoLink se encuentra en `src/app/home/admin/echolink` y reutiliza traducciones, controles compartidos y API administrativa. Las migraciones de instalación preparan las comprobaciones del dialplan en `[allstar-public]` sin modificar `[radio-secure]`.

### Construcción y publicación 

> **NOTA:** Solo el desarrollador original puede hacer el push oficial con la firma del repositorio, si usted hace un fork debe pedir la autorizacion correspondiente pull request, se revisará todo el código cambiado/adicionado.


`npm run build` genera el frontend en `internal/web/dist`, que Go incrusta mediante `go:embed`. `make dist` **publica realmente**, exige un árbol Git limpio y no es una comprobación de compilación.

El pie de la aplicación muestra la versión inyectada en el backend: `make run` y `make dev-backend` utilizan `git describe --tags --always`; los paquetes reciben la versión del tag mediante GoReleaser. Un `go run` sin inyectar versión muestra `dev`.

La publicación completa sigue estos pasos:

1. Publica la rama actual y calcula/crea/sube el siguiente tag semver.
2. Ejecuta `goreleaser release --clean`; sus hooks ejecutan `go mod tidy`, `npm ci` y `npm run build`.
3. Genera paquetes `.deb` Linux `amd64` y `arm64` con CGO desactivado.
4. Copia al repositorio local, conserva hasta tres versiones por arquitectura, regenera/firma índices y hace push a `jjsainzc/repo-deb`.

```bash
make dist
make dist VERSION=v1.2.0
make index-repo
make push-repo
```

Los comandos son alternativas según la tarea; no ejecutes `make dist` dos veces para una misma publicación. Antes de anunciar una versión, comprueba que `https://jjsainzc.github.io/repo-deb/Packages` la incluya y que `apt-cache policy amerimon` muestre el `Candidate` esperado en el cliente.

#### Limpieza posterior a la distribución

Usa `make clean-web-dist` para eliminar únicamente el frontend generado de `internal/web/dist`, conservando `.gitkeep` para `go:embed`. La llamada automática desde `make release` está comentada en el Makefile actual: la limpieza es manual. Los binarios y paquetes de `dist/` ya contienen el frontend y no se modifican. Conserva el frontend si necesitas diagnosticar una distribución fallida. Para volver a ejecutar desde fuentes con interfaz embebida, usa `make run` o ejecuta antes `npm run build`.

La limpieza no elimina ni regenera tests. El script se verifica con `node --test scripts/clean-web-dist.test.cjs`.

## Documentación operativa

Para configuración de nodos, puertos, SQLite, proxy, respaldos, operaciones administrativas y diagnóstico, consulta el [Manual de Operación Técnica](public/assets/docs/technical-operations-manual.es.md).

Los originales de los manuales técnicos y Listen Live en español e inglés se mantienen en `public/assets/docs`; Angular los publica bajo `assets/docs`. Las guías complementarias se conservan en `docs/manuals`, fuera de la distribución web. Los manuales técnicos incorporan las instrucciones operativas de hostname, Nextion, PJSIP y audio. La depuración, integración Angular, ejecución local, pruebas y publicación se documentan aquí.
