# Manual de Operación Técnica de Amerimon

## 1. Alcance

Amerimon es una aplicación web para consultar AllStarLink, administrar nodos Allmon, escuchar audio en vivo y ejecutar operaciones autorizadas en Asterisk. El frontend Angular se compila y se incrusta en un único servicio Go.

Este manual describe la instalación, configuración y operación de la distribución Debian. Los usuarios instalan y actualizan Amerimon con APT; no necesitan compilar el código ni ejecutar tareas de publicación.

Amerimon consulta Asterisk mediante AMI. Su postinstalador prepara los módulos y contextos necesarios para Listen Live. El administrador puede cambiar explícitamente la identidad y las listas de acceso del nodo principal, así como administrar usuarios IAX y extensiones PJSIP desde sus páginas específicas. No se reconfiguran automáticamente los demás nodos ni los usuarios de telefonía.

Este manual está dirigido a administradores y operadores. Para una instalación nueva, sigue primero la sección 2; después consulta la sección 3 para los puertos, la sección 4 para administrar el servicio y la sección 5 para integrar Asterisk. La sección 6 explica las funciones de administración de la web; las secciones restantes cubren datos, respaldos, registros, mantenimiento y diagnóstico.

## 2. Instalación Debian

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
| `/var/lib/amerimon/terminal-credentials.key` | Clave maestra para cifrar credenciales SSH, permisos `0600` |
| `/lib/systemd/system/amerimon.service` | Unidad systemd |
| `/usr/local/bin/astdb.sh` | Actualización de la base de nodos AstDB |

Las configuraciones `amerimon.env` y `amerimon.yaml` se instalan como `config|noreplace`; una actualización conserva los archivos locales existentes.

## 3. Componentes y puertos

| Componente | Dirección predeterminada | Uso |
| --- | --- | --- |
| HTTP de Amerimon | `0.0.0.0:8082` en el paquete | Frontend embebido y API REST `/mon` |
| Audio EAGI | `127.0.0.1:8086` | Entrada local de audio para Listen Live |
| Asterisk AMI | `127.0.0.1:5038` | Consultas y operaciones de Asterisk |

El puerto `8086` debe estar libre al iniciar Amerimon. El proceso EAGI y la conexión AMI son locales al servidor. Si no se instala Apache o Nginx, Amerimon sigue funcionando directamente en `http://<IP-del-servidor>:8082`; sin proxy no existe la ruta `/amerimon` ni HTTPS.

## 4. Servicio systemd

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

## 5. Configuración de Asterisk y nodos

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

En la instalación Debian, `postinstall.sh` intenta leer el primer usuario AMI que no sea `[general]` en `/etc/asterisk/manager.conf`. Solo agrega `AMI_USER` y `AMI_SECRET` si todavía no están definidos en `amerimon.env`. Si no puede leerlos, Amerimon sigue instalado, pero las consultas y acciones AMI no funcionarán hasta completar esas variables. El administrador también puede actualizar el secret y el bind address del usuario AMI configurado desde **Admin → Nodo principal**; esta acción conserva el archivo y sus ajustes personalizados, crea respaldo, actualiza `amerimon.env` y ejecuta `manager reload` sin reiniciar Asterisk.

Amerimon no crea usuarios Asterisk. El usuario AMI existente debe tener los permisos necesarios para las acciones que se usan, incluida la ejecución de comandos y `Originate` para Listen Live. Las redes `permit`/`deny` agregadas desde la web son aditivas y no sustituyen ACL personalizadas existentes.

Los pasos de configuración se ejecutan de forma independiente: un fallo en credenciales, directorios, clave de terminal, una operación de systemd, proxy o un parche Asterisk se informa y no impide intentar los demás. En particular, un reinicio fallido de Amerimon no omite el temporizador astdb ni el proxy. `install.sh` reutiliza este postinstalador y un estado final inactivo solo genera un aviso; los fallos de compilación o instalación de archivos necesarios siguen siendo fatales. Una instalación terminada no garantiza que todas las funciones estén operativas: revisa los avisos.

Si existe `/etc/asterisk`, el bloque de Listen Live guarda el estado de `modules.conf` que encuentra como `modules.conf.bak-listen-live` y habilita los módulos presentes `res_speech.so`, `res_agi.so`, `app_chanspy.so`, `app_mixmonitor.so`, `app_verbose.so`, `func_config.so`, `func_strings.so`, `func_logic.so` y `func_cut.so`. Si el paso PJSIP ya modificó el archivo, ese respaldo intermedio incluye los cambios PJSIP; el original previo a esos cambios está en `modules.conf.bak-pjsip`. `app_verbose.so` proporciona las aplicaciones `Verbose` y `Log` del contexto: si no está cargado, Asterisk termina la llamada antes de llegar a ChanSpy. Convierte una directiva `noload` de esos módulos en `load` o agrega `load => ...` al final. Si existe `extensions.conf`, guarda el estado previo al dialplan de Listen Live en `extensions.conf.bak-listen-live` y agrega los contextos `[listen-live-spy]`, `[listen-live-eagi]` y `[amerimon-listen-node]` cuando faltan. Solo migra los bloques `nodo` antiguos reconocidos de Amerimon (SimpleUSB o Local/pseudo), sustituyéndolos por una única redirección al contexto nuevo; los bloques personalizados se conservan y generan un aviso. Antes de una migración se guarda también `extensions.conf.bak-listen-live-rxchannel`.

Si Asterisk ya está activo, el hook intenta cargar esos módulos y ejecutar `dialplan reload`; no reinicia el servicio Asterisk. Si un módulo no existe en esa distribución, registra un aviso y continúa la instalación de Amerimon. Las copias `.bak-listen-live` permiten revisar o restaurar manualmente el estado previo.

### PJSIP para teléfonos SIP

El postinstalador habilita e inicia `amerimon-pjsip-firewall.service` con `systemctl enable --now` y comprueba que esté activo antes de configurar PJSIP. Su estado normal es `active (exited)` porque es una unidad `oneshot` con `RemainAfterExit=yes`. Si ya estaba activa, vuelve a verificar las reglas con el script actualizado sin reiniciar la unidad ni Asterisk. Si falla el inicio o la comprobación, aborta con un diagnóstico para consultar el journal del servicio.

El postinstalador también integra `packaging/pjsip-modules-patch.conf` y `packaging/pjsip-extensions-sip-phones-patch.conf`. Si falta alguno de los archivos, crea `modules.conf` con `[modules]` y `autoload=yes`, o crea `extensions.conf`. Agrega todas las directivas de módulos que faltan, sin condicionar su incorporación a la presencia del archivo `.so`. Para los módulos que el parche habilita, convierte `noload` en `load` y elimina duplicados. Conserva los demás módulos y ajustes personalizados e incluye las directivas `noload` del parche cuando no hay una directiva existente para esos módulos. En `pjsip.conf`, agrega `[transport-udp]` (`type=transport`, `protocol=udp`) con un bind a una IPv4 privada de una interfaz activa. Prefiere una dirección RFC1918 de la interfaz de la ruta predeterminada; si esa interfaz no tiene dirección privada, elige la primera dirección de `10/8`, `172.16/12` o `192.168/16` encontrada, incluidas VPN y túneles. Reemplaza los binds `0.0.0.0` existentes, conservando su puerto; agrega un bind si falta, pero no cambia binds específicos ni ejemplos comentados. Si modifica el archivo, guarda el original en `pjsip.conf.bak-pjsip`. Solo utiliza `0.0.0.0` si no hay IPv4 privada. Si no puede identificar ninguna dirección utilizable, aborta sin habilitar PJSIP. Este paso no descarga paquetes Asterisk.

Antes de cargar PJSIP, `pjsip-network.sh protect` verifica reglas de bloqueo de UDP/5060 hacia direcciones IPv4 locales no privadas. Reutiliza firewalld activo; en su ausencia usa iptables instalado. Si solo existe firewalld detenido, lo habilita; si no hay ninguno, instala iptables con APT. No vacía reglas ni cambia políticas generales o puertos distintos de UDP/5060. La cadena `AMERIMON-PJSIP` en `raw/PREROUTING` permite destinos RFC1918, loopback y link-local, y descarta los demás destinos de tipo `LOCAL`, antes de cualquier ACCEPT de conexiones existentes. Así cubre nuevas IP públicas de interfaces físicas, VPN o túneles sin guardar una lista fija y no afecta al tráfico reenviado. Firewalld recibe reglas runtime y permanentes. `amerimon-pjsip-firewall.service` reaplica y verifica la protección al arrancar, después de la red y antes de Asterisk, que requiere ese servicio; un fallo bloquea la configuración o el arranque dependiente. Al desinstalar Amerimon se deshabilita el servicio, pero no se eliminan las reglas de firewall existentes. Si la IP pública está en un router NAT, también hay que cerrar allí el port-forward UDP/5060: el servidor ve el destino privado después de esa traducción y un bind privado no la impide.

#### Comprobar el firewall y el bind después de instalar

Ejecuta en el servidor:

```bash
systemctl status amerimon-pjsip-firewall.service --no-pager
journalctl -u amerimon-pjsip-firewall.service -b --no-pager
iptables -t raw -L -n -v
iptables -t raw -S AMERIMON-PJSIP
asterisk -rx 'pjsip show transport transport-udp'
netstat -uln
```

Con iptables, debe existir el salto UDP/5060 desde `PREROUTING` hacia `AMERIMON-PJSIP`, los `RETURN` para destinos privados y el `DROP` para otros destinos locales. Los contadores de `DROP` muestran tráfico descartado, no necesariamente un ataque. `iptables -L -v` sin `-t raw` muestra la tabla `filter` y puede estar vacía aunque la protección esté funcionando. Si se usa firewalld, consulta `firewall-cmd --direct --get-all-rules` y `firewall-cmd --permanent --direct --get-all-rules` en su lugar.

El servicio debe estar habilitado y `active (exited)`. En instalaciones anteriores puede estar `inactive (dead)` porque las reglas se aplicaron ejecutando directamente el script; esto no elimina las reglas que ya existen. Para activar la unidad y verificarla sin reiniciar Asterisk:

```bash
systemctl enable --now amerimon-pjsip-firewall.service
systemctl status amerimon-pjsip-firewall.service --no-pager
```

Compara el bind del archivo `pjsip.conf` con el transporte activo reportado por Asterisk y el socket de `netstat`: son comprobaciones distintas. Si Asterisk informa `allow_reload : false`, una respuesta `Module 'res_pjsip.so' reloaded successfully` no confirma que se haya cambiado el bind del transporte existente. Amerimon no habilita automáticamente `allow_reload` ni reinicia Asterisk. Si sigue activo `0.0.0.0:5060`, verifica primero el bloqueo público y aplica el bind privado mediante un reinicio manual de Asterisk durante una ventana de mantenimiento; ese reinicio interrumpe conexiones y sesiones de escucha. La protección descrita cubre UDP/IPv4 en el puerto 5060, no otros transportes, puertos ni IPv6.

#### Opciones habituales de endpoint PJSIP

Las siguientes opciones ilustran ajustes que suelen revisarse en endpoints PJSIP, por ejemplo, teléfonos detrás de NAT. Amerimon no las aplica automáticamente y no forman una plantilla universal: confirma que cada opción esté admitida y sea adecuada para tu versión de Asterisk, transporte y dispositivo. Consulta la documentación de la versión instalada antes de editar `pjsip.conf`.

| Opción | Uso |
| --- | --- |
| `rtp_symmetric=yes` | Envía paquetes RTP de vuelta a la misma dirección IP y puerto desde los que se recibieron. Ayuda cuando NAT hace que la dirección anunciada por el dispositivo no sea alcanzable. |
| `force_rport=yes` | Responde al puerto UDP de origen real del paquete SIP, en vez de confiar en el puerto indicado dentro de la solicitud. Es útil cuando un router NAT cambia el puerto de origen. |
| `rewrite_contact=yes` | Sustituye la dirección `Contact` anunciada durante el registro por la dirección y el puerto desde los que Asterisk recibió el paquete. Evita intentar contactar una IP privada que no es alcanzable desde Asterisk. |
| `ice_support=yes` | Habilita ICE para negociar una ruta de medios entre los extremos, cuando ambos lo admiten. ICE puede evaluar candidatos de red, incluidos los obtenidos con STUN/TURN; no configura ni reemplaza esos servicios. |
| `direct_media=no` | Evita que Asterisk intente sacar sus medios del flujo de la llamada: el audio continúa pasando por Asterisk. Puede ser necesario para grabaciones, conferencias, transferencias asistidas o redes con NAT estricto. En documentación antigua puede aparecer como `canreinvite=no`. |
| `disable_direct_media_on_nat=yes` | En versiones que admiten esta opción, desactiva los medios directos al detectar NAT aunque `direct_media` esté habilitado. Comprueba su disponibilidad y relación con `direct_media` en tu versión. |
| `use_avpf=no` | Desactiva AVPF y usa el perfil RTP/AVP convencional cuando ese sea el perfil negociado por el teléfono. No es una opción genérica para endpoints WebRTC ni sustituye la configuración de códecs. |
| `inband_progress=no` | No genera tonos de progreso (como timbre o llamada en curso) dentro del audio; deja que la señalización SIP y el dispositivo los gestionen fuera de banda. Confirma el comportamiento esperado de tus teléfonos. |
| `tos_audio=ef`, `tos_video=af41` | Marca paquetes de audio y video con DSCP Expedited Forwarding y Assured Forwarding, respectivamente. Estas marcas solo priorizan tráfico si los switches y routers de la ruta están configurados para respetarlas. |
| `cos_audio=5`, `cos_video=4` | Solicita prioridad CoS/IEEE 802.1p de capa 2 para audio y video. Solo tiene efecto en enlaces Ethernet/VLAN y equipos que admitan y apliquen esas marcas. |
| `allow_subscribe=no` | Rechaza las solicitudes SIP `SUBSCRIBE` del endpoint, por ejemplo, cuando una troncal o teléfono no necesita BLF ni notificaciones de buzón. |
| `media_encryption=no` | Desactiva SRTP; el audio y el video viajan sin cifrado. No lo uses como decisión predeterminada en redes no confiables; configura transporte y cifrado compatibles cuando necesites proteger la señalización y el audio. |

Estas opciones son independientes de la protección de UDP/5060 descrita arriba. Un bind privado y las reglas de firewall no sustituyen la seguridad de credenciales, TLS/SRTP ni las ACL de Asterisk.

La sección `[sip-phones]` enruta las llamadas PJSIP por un canal Local antes de entregarlas a `app_rpt`, para evitar el enrutamiento PJSIP directo que puede hacer que Asterisk falle:

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

Si esos contextos ya existen sin una ruta `_X.`, el instalador agrega las rutas del parche dentro de la sección existente. Las dos rutas exactas del parche anterior se migran para identificar el RX SIP. Las rutas `_X.` personalizadas no se modifican ni reciben prioridades duplicadas: se conservan y se registra un aviso si requieren integración manual. Antes de cambiar archivos existentes, crea `modules.conf.bak-pjsip` y/o `extensions.conf.bak-pjsip` si esos respaldos todavía no existen. Los archivos recién creados no se respaldan.

Si Asterisk está activo, el postinstalador carga primero Sorcery, PJPROJECT y RTP, después los recursos base de PJSIP y finalmente el controlador de canal. Solo considera activos los módulos en estado `Running`, no `Not Running`. Reintenta automáticamente los pendientes mientras otras cargas resuelvan dependencias, sin limitarse a dos pasadas. Si deja de avanzar y faltan módulos requeridos, termina con error y muestra la respuesta de Asterisk; no declara completada la activación ni elimina la configuración o sus respaldos. Verifica `[transport-udp]` en Asterisk y confirma con `netstat` el socket UDP de la dirección y puerto configurados. Si cambió el bind o falla alguna comprobación, ejecuta `module reload res_pjsip.so` y verifica de nuevo. Si sigue sin aparecer el socket esperado, informa la respuesta de Asterisk sin reiniciar el servicio; algunos transportes requieren un reinicio manual para cambiar de bind. El paquete Debian incluye `net-tools`, `iproute2` y la alternativa `iptables | firewalld`. `res_pjsip_publish_asterisk.so` se deshabilita con `noload`: intercambia eventos PUBLISH entre servidores y no es necesario para extensiones SIP, teléfonos PJSIP ni Listen Live. Esto evita mensajes de carga fallida del módulo opcional durante la instalación y el inicio de Asterisk. Si tu instalación necesita PUBLISH entre servidores, configura primero un Entity ID válido en Asterisk y cambia su directiva `noload` a `load` en `modules.conf`; entonces diagnostica cualquier fallo con `asterisk -rx 'core show settings'` y el registro de Asterisk. Si Asterisk está detenido, las directivas persistentes se aplicarán al próximo inicio. Las directivas de dialplan se incorporan durante el postinstall; revisa sus avisos si existe una ruta personalizada.

### Selección y sintaxis de nodos Allmon

Las actualizaciones no reemplazan `/etc/amerimon/allmon.ini`: el instalador lo crea solo si falta. El editor de nodos Allmon conserva las secciones que no administra y genera una copia `.amerimon.bak` antes de guardar.

#### Conectar a un servidor remoto mediante hostname

El nombre debe resolver desde el servidor donde se ejecuta el backend de Amerimon, no solo desde el navegador o el equipo desde el que administras el servidor. El comando `hostname` muestra el nombre local del equipo, pero no lo registra por sí solo en el DNS de la red. Para una LAN pequeña, la opción recomendada es mDNS con Avahi y un nombre completo como `equipo.local`; para varias subredes o VLAN, configura DNS local.

##### Opción recomendada: mDNS / Avahi

1. **En el servidor Asterisk remoto**, comprueba el nombre y el servicio mDNS:

   ```bash
   hostname
   systemctl status avahi-daemon --no-pager
   ```

   En Debian, Ubuntu o Raspberry Pi OS, instala y habilita Avahi si hace falta:

   ```bash
   sudo apt update
   sudo apt install avahi-daemon
   sudo systemctl enable --now avahi-daemon
   ```

   Para cambiar el nombre, usa uno único en la LAN con `sudo hostnamectl set-hostname equipo`. Solo si lo cambias, revisa `sudoedit /etc/hosts` y actualiza la entrada local antigua (a menudo `127.0.1.1`), conservando `localhost`; después ejecuta `sudo systemctl restart avahi-daemon`. Si el nombre ya es correcto y `equipo.local` resuelve, no cambies nada.

2. **En el servidor del backend Amerimon**, verifica la resolución; probar desde una estación SSH no basta:

   ```bash
   getent ahosts equipo.local
   ```

   Si no resuelve, en Debian/Ubuntu que usan Avahi y NSS instala los componentes que falten:

   ```bash
   sudo apt update
   sudo apt install avahi-daemon avahi-utils libnss-mdns
   sudo systemctl enable --now avahi-daemon
   ```

   Comprueba `grep '^hosts:' /etc/nsswitch.conf`, `avahi-resolve-host-name -4 equipo.local` y `getent ahosts equipo.local`. Una línea habitual de NSS es `hosts: files myhostname mdns4_minimal [NOTFOUND=return] dns`; es un ejemplo, no la reemplaces a ciegas: conserva los módulos de tu distribución. En sistemas con `systemd-resolved`, mDNS puede gestionarse allí en vez de Avahi/NSS.

   Amerimon resuelve nombres terminados en `.local` mediante `getent ahosts`, que consulta NSS/Avahi sin usar DNS público. Otros hostnames usan el resolvedor de Go y las direcciones IP se usan directamente. El paquete Debian declara `libc-bin`, `libnss-mdns` y `avahi-daemon` como dependencias. Si el log sigue mostrando `lookup ...local on 1.1.1.1:53`, comprueba la versión instalada y actualiza con APT según la sección 2; después revisa el log del servicio reiniciado.

3. **Comprueba la red y AMI.** mDNS suele funcionar solo dentro de la misma LAN y usa multicast UDP `5353`; Wi-Fi, firewall o aislamiento de clientes pueden bloquearlo. No suele cruzar subredes o VLAN sin configuración adicional. Tras resolver el hostname, comprueba el puerto AMI:

   ```bash
   nc -vz -w 5 equipo.local 5038
   ```

   Si el nombre resuelve, pero TCP no responde, revisa AMI, el firewall y las restricciones de acceso del servidor remoto. Una conexión TCP exitosa no valida usuario ni contraseña.

4. **Configura el nodo remoto** en **Admin → configuración de Allmon**, guardando el hostname completo en **Host** y el puerto por separado:

   ```text
   Host:   equipo.local
   Puerto: 5038
   ```

   Usa las credenciales AMI de ese servidor. No incluyas `http://`, `ssh://` ni el puerto en Host. Amerimon no agrega `.local` automáticamente. Si cambiaste `ami.host` en el YAML en vez de configurar un servidor remoto de Allmon, reinicia el backend para aplicar el cambio.

##### Alternativas: DNS local y `/etc/hosts`

Para redes con varias subredes/VLAN o donde mDNS no sea apropiado, configura DNS local en el router o servidor de red; por ejemplo, registros asociados a reservas DHCP y una zona `home.arpa`. No uses `.local` como zona DNS unicast, ya que está reservada para mDNS. Haz que DHCP entregue el DNS local a todos los clientes, incluido el backend Amerimon, y verifica desde ese backend con `getent ahosts equipo.home.arpa`. Para usar solo `equipo`, el resolvedor debe tener el nombre o un dominio de búsqueda como `home.arpa`. No edites directamente un `/etc/resolv.conf` generado por el sistema; configura el gestor de red o el router. DNS públicos como `1.1.1.1` y `8.8.8.8` no conocen los nombres privados de la LAN.

Como último recurso, puedes agregar una entrada en `/etc/hosts` **del servidor del backend**:

```text
192.168.1.20 equipo
```

Esto permite usar `equipo`, pero depende de una IP fija o reservada; actualiza la entrada si cambia la dirección y replícala en cada equipo que deba resolver el nombre.

| Síntoma | Qué revisar |
| --- | --- |
| SSH resuelve el host, pero Amerimon no | Windows y Linux pueden usar resolvedores distintos; prueba desde el servidor del backend. |
| `equipo` falla y `equipo.local` funciona | Guarda el nombre completo con `.local`. |
| El nombre resuelve, pero TCP/5038 falla | Revisa la escucha AMI, firewall, ruta de red y permisos. |
| TCP responde, pero AMI rechaza el acceso | Revisa usuario, contraseña y configuración AMI remota. |
| Funciona en una LAN, pero no entre VLAN | Usa DNS local o configura explícitamente la infraestructura mDNS. |

No abras AMI a Internet ni desactives sus controles de acceso.


La búsqueda de `allmon.ini` sigue este orden:

1. `sources.allmon_ini_path`, si se configuró explícitamente.
2. `/etc/allmon3/allmon.ini`.
3. `/var/www/html/supermon/allmon.ini`.
4. `sources.allmon_ini_own_path`; predeterminado `/etc/amerimon/allmon.ini`.

Los nodos locales se obtienen de `/etc/asterisk/rpt.conf` (o `ami.rpt_config_path`), incluido el principal identificado por `<nodo> = radio@127.0.0.1/<nodo>,NONE` en `[nodes]`. Se excluyen por defecto como servidores los nodos de `privatenodes.txt` y los clasificados como privados (cuatro dígitos). Los privados locales de cuatro dígitos marcados **Mostrar como servidor en AllStarLink** sí crean su tabla y opción en el selector. Sus conexiones siguen apareciendo en la tabla del nodo al que están conectadas, sea el principal u otro servidor. El principal no se elige por orden ni por esta casilla: si varias entradas no privadas cumplen la regla loopback, se registra el error de ambigüedad. Los nodos locales usan las credenciales AMI globales de Amerimon. Las entradas Allmon dirigidas a loopback o al host AMI global no sustituyen estos nodos ni sus credenciales. Las instancias de servidores remotos conservan su configuración Allmon. Esta selección se comparte entre la tabla, el sondeo de actividad y las acciones sobre nodos.

En el monitor AllStarLink, la tabla del nodo principal aparece primero; las restantes se ordenan por número. La cabecera de cada tabla muestra `número - descripción - IP/hostname`; el principal conserva su etiqueta «Nodo principal» y las remotas no llevan el prefijo «Nodo remoto». La descripción procede de `astdb.txt` y la dirección del host AMI global para los nodos locales o del campo `host` de Allmon para los remotos (sin puerto). Se muestra la IP o el hostname según lo configurado, sin consultas DNS; los datos ausentes se omiten, incluso cuando el servidor está desconectado.

En **Actualizar cada → Tiempo real**, el navegador abre el WebSocket `/mon/allstarlink/activity/live` en el mismo servidor y puerto que los demás WebSockets de Amerimon. Un sondeo AMI compartido consulta cada 250 ms el COS local (`NodeStat`, `SignalOnInput`) y la recepción activa de cada enlace (`SawStat`, indicador RX); las consultas de un servidor no se solapan ni bloquean las de otros servidores. El WebSocket publica el estado cada 250 ms. Si no conecta, la vista consulta ese mismo estado por HTTP cada 500 ms. El estado incluye también los nodos conectados observados por `SawStat`: al detectar un cambio, la vista vuelve a consultar solo la tabla del servidor afectado para mostrar nuevas conexiones IAX/IAXRpt o retirar las desconectadas. El resaltado y el indicador verde intermitente de la cabecera se apagan en la siguiente actualización que detecta COS/RX desactivado, sin esperar al vencimiento del historial. Los nodos configurados con COS activo también se resaltan en las tablas donde están conectados. La latencia incluye los tiempos de respuesta AMI y de red; los estados sin actualizar caducan tras tres segundos. El historial de última actividad se mantiene por separado. Al elegir otro intervalo, se detiene el monitoreo de la vista.

El sondeo de tiempo real consulta también `core show channels concise` y publica los endpoints PJSIP con canales en estado `Up` en `sip_connections`, tanto por WebSocket como por HTTP. Al iniciarse o terminar una llamada SIP se vuelve a consultar la tabla del servidor afectado, aunque sus enlaces IAX no hayan cambiado. Un contacto registrado sin llamada activa no provoca una conexión SIP. Si falla la consulta de canales, se registra el error y se omite el estado SIP del servidor en esa actualización; no se interpreta el fallo como una desconexión.

El resaltado SIP y el indicador de su servidor usan los eventos `ChannelTalkingStart`/`ChannelTalkingStop` de `TALK_DETECT`, no el estado `Up`, los paquetes RTP de silencio ni el PTT de `SawStat` (que no refleja el VOX telefónico en app_rpt 3.10.8). El parche habilita `TALK_DETECT(set)=250` en el canal PJSIP antes de `Dial(Local/...)`: tras 250 ms de silencio publica el fin de voz. Se conserva el umbral de voz predeterminado de Asterisk. Una sesión AMI compartida por servidor recibe los eventos de cada canal, identifica el endpoint PJSIP y el nodo destino (`Exten`) y publica `SIP/<endpoint>` como actividad por HTTP y WebSocket. Los indicadores se apagan con el silencio, Hangup, caducidad de la conexión o pérdida de la sesión AMI. Las llamadas simultáneas se siguen por canal y Uniqueid.

Al instalar o actualizar, solo se migran automáticamente las rutas exactas de los parches anteriores (anónimo o con endpoint nombrado), con respaldo en `extensions.conf.bak-pjsip`. Las rutas personalizadas se conservan y se avisa si falta el detector. El parche de módulos carga `func_talkdetect.so`; AMI requiere permiso de lectura `call` para los eventos de voz y Hangup. No se modifican automáticamente permisos AMI personalizados. Se mantienen el canal Local, el modo Pv y la configuración RTP. Después de aplicar el dialplan y cargar el módulo, recarga con `asterisk -rx 'dialplan reload'`, reinicia solo Amerimon si actualizaste su backend y reconecta el teléfono. Comprueba al hablar y guardar silencio que `/mon/allstarlink/activity?recent=true` agrega y retira la actividad `SIP/<endpoint>`. Una llamada ya abierta antes de habilitar el detector debe reconectarse; tras reconectar AMI se espera el siguiente inicio de voz.

En tiempo real, el último enlace que tuvo actividad conserva la primera posición de su tabla al cesar el audio, sin conservar el resaltado ni el indicador intermitente. Una nueva actividad tiene prioridad; el campo de orden elegido se usa para desempatar. Al salir del modo en tiempo real se recupera el orden elegido.

Al cambiar de pantalla se cierra el WebSocket y se detienen los temporizadores de la vista, pero se conserva la opción de actualización durante la navegación. Al volver al monitor, si estaba seleccionado **Tiempo real**, se abre automáticamente una nueva conexión.

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
- En **Administración → Favoritos**, un único campo **Búsqueda** consulta AstDB por número de nodo, indicativo o descripción. Una coincidencia selecciona el nodo automáticamente y, si hay varias, puedes elegir el destino; el favorito guarda el número seleccionado.
- La misma búsqueda incluye EchoLink cuando la AMI del servidor principal confirma que `chan_echolink.so` está cargado y `echolink dbdump` devuelve registros válidos `número|indicativo|IP`. Puedes buscar por el número original o por el número de conexión con prefijo `3`, o por indicativo. Por ejemplo, `996546|W2LRC-R|165.227.99.201` se ofrece y guarda como `3996546`, con descripción sugerida `W2LRC-R EchoLink 165.227.99.201`. Sin módulo o sin registros no se añaden destinos EchoLink; los fallos de consulta se registran en el log y la búsqueda AstDB sigue disponible. No se carga ni reconfigura el módulo automáticamente.

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

#### Nodo ya enlazado: cambiar modo o permanencia

En **Conectar nodo remoto**, el servidor seleccionado, el nodo remoto, el modo (**Transceive**, **Monitor** o **Local Monitor**) y la casilla **Conexión permanente** definen la modalidad solicitada. Después de confirmar, Amerimon consulta el estado real por AMI:

- **Mismo modo y misma permanencia verificable:** muestra un aviso de que el nodo ya está conectado o conectándose con esa modalidad, sin desconectarlo ni enviar otro comando de conexión. La API devuelve HTTP `409`.
- **Modo o permanencia diferentes:** desconecta únicamente el enlace directo de ese nodo remoto en el servidor seleccionado mediante `ilink 11`, que también permite retirar enlaces permanentes. Los otros enlaces y servidores no se desconectan.
- **Permanencia anterior no verificable:** el aviso de confirmación explica que se desconectará y reconectará ese enlace con los valores solicitados. No se presupone que una conexión sin registro sea temporal.

Por ejemplo, cambiar un enlace **Transceive permanente** a **Monitor temporal** primero retira el enlace anterior y luego envía `ilink 2`; cambiar únicamente la permanencia también requiere reconectar. Amerimon espera hasta 15 segundos a que `XStat` confirme que el enlace anterior desapareció, vuelve a validar la topología completa y solo entonces envía el nuevo comando. Si falla la desconexión, la consulta o la comprobación de ausencia, no inicia la nueva conexión. Tampoco conecta si después aparece otra ruta que produciría un bucle. Un enlace retirado no se restaura automáticamente si la reconexión falla; revisa **Registros Asterisk** antes de reintentar.

`XStat` informa el modo, pero no la permanencia. Amerimon contrasta el estado actual con el último comando de enlace compatible conservado en **Registros Asterisk**. Una desconexión posterior invalida esa referencia. Las conexiones creadas fuera de Amerimon o sin un registro compatible se tratan como permanencia desconocida; los cambios externos de permanencia sin cambio de modo no pueden detectarse mediante `XStat`.

Este comportamiento se aplica tanto a `POST /mon/asterisk/commands` como a `POST /mon/asterisk/connections`. La opción explícita **Desconectar todos antes de conectar** sigue siendo distinta: si está marcada, primero retira todos los enlaces RPT del servidor seleccionado, no solo el nodo solicitado. Déjala desmarcada para cambiar únicamente la modalidad de un enlace existente.

#### Prevención de bucles entre nodos visibles

Todos los nodos se consideran pares: no se exige un hub ni se limita el número de saltos. Antes de iniciar un enlace, Amerimon consulta por AMI el `XStat` de los nodos locales y remotos con seguimiento habilitado, además del origen solicitado. Construye un grafo no dirigido con sus enlaces `Conn:` establecidos o en conexión y la alcanzabilidad anunciada en `LinkedNodes:`. `LinkedNodes: <NONE>` es el marcador de Asterisk para una lista vacía, no una ruta inválida; se siguen comprobando los enlaces `Conn:` presentes en la misma respuesta. Un enlace directo existente en el origen se procesa según las reglas de cambio de modalidad anteriores; si se retira, el grafo se consulta de nuevo. Si origen y destino todavía pertenecen a la misma componente, rechaza el nuevo enlace con HTTP `409`, mostrando la ruta visible o pendiente que lo impide. También rechaza conexiones del nodo consigo mismo y conexiones espejo que solo aparecen desde el otro extremo. Las rutas de `LinkedNodes:` indican alcanzabilidad, no necesariamente cada salto físico. `XStat` se solicita mediante AMI (`Action: RptStatus`, `Command: XStat`, `Node: <nodo>`); no mediante el comando CLI `rpt xstat`.

Esta validación se aplica en el servidor a `POST /mon/asterisk/commands` para Connect, Monitor y Local Monitor, tanto temporales como permanentes, y a todos los modos de `POST /mon/asterisk/connections`. Los rechazos de validación quedan en **Registros Asterisk**. El monitor muestra el motivo y retira la conexión pendiente de la interfaz. El historial SQLite no se toma como prueba de una conexión activa: las consultas AMI se realizan de nuevo antes de enviar el comando.

Al confirmar una conexión, su fila aparece inmediatamente al principio de su tabla con fondo rojo y **Conectando…** en **Enlace**, incluso durante la desconexión previa. Las filas pendientes se muestran aunque no coincidan con los filtros de servidor o búsqueda, sin cambiar esos filtros. El monitor sigue consultando hasta observar el enlace establecido, incluso sin actualización periódica seleccionada. Los estados `CONNECTING` recibidos del servidor se presentan igual. Si la solicitud falla, se muestra el error y se elimina la fila pendiente, sin retirar enlaces ya establecidos.

Si no se puede consultar alguno de los nodos del alcance o su respuesta no contiene una lista de enlaces interpretable, se omite esa respuesta y se comprueba la información disponible de los demás nodos y las reservas pendientes. La topología incompleta no bloquea por sí sola el nuevo comando ni muestra un error de validación; el diagnóstico se conserva en el log del servidor. Si no se puede cargar el inventario de topología, se intenta consultar al menos el origen seleccionado. Los autoenlaces y las rutas visibles o pendientes que sí se detecten siguen rechazándose con HTTP `409`. **Con nodos inaccesibles no se puede garantizar la ausencia de bucles.** Un fallo de la operación AMI solicitada sí se muestra como error. Los nodos excluidos mediante **Seguimiento** no forman parte de las consultas; las rutas que otros nodos anuncien hacia ellos sí pueden detectarse. Desconectar sigue sin requerir una topología completa. Con `disconnect_before_connect`, primero se confirma la desconexión del origen y después se vuelve a comprobar la topología disponible; una solicitud de autoenlace se rechaza antes de desconectar nada. Las comprobaciones de desconexión del enlace anterior siguen siendo obligatorias para cambiar su modalidad.

En el monitor, los fallos de consulta de nodos, incluido el principal, no muestran el mensaje de error del nodo inaccesible. El grupo conserva el estado de reintento en segundo plano cada 5 segundos, sin bloquear los demás servidores. Los errores generales de configuración y los de acciones solicitadas por el usuario siguen visibles.

Amerimon serializa las operaciones de enlace de este proceso; una solicitud concurrente recibe HTTP `409` y debe reintentarse. Tras aceptar AMI un enlace, conserva una reserva en memoria durante el timeout de conexión (15 segundos por defecto) hasta observarlo en `XStat`, para evitar que otra solicitud cierre un bucle durante el establecimiento. Las reservas no sobreviven a un reinicio; las conexiones visibles se consultan nuevamente. La protección no puede garantizar rutas ocultas, cambios simultáneos realizados fuera de Amerimon, ni reconexiones permanentes futuras de Asterisk después de vencer esa reserva.

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

### Reconexión automática de nodos desde una fuente

Para fuentes remotas Allmon se usa el mismo criterio AMI que en las conexiones manuales: host, puerto y usuario son obligatorios; una contraseña vacía configurada en Allmon no se rechaza antes del intento. Asterisk decide si acepta esas credenciales. No se recomienda usar contraseñas vacías.

El destino es libre: no necesita aparecer en el catálogo ni en los servidores AllStarLink. Solo se valida su formato numérico y que no coincida con la fuente. El combo y la comprobación de configuración AMI corresponden únicamente al nodo fuente. Si falla guardar una entrada activada, el error identifica la fuente y el destino de esa solicitud; los errores de carga del combo se presentan por separado. No se comprueba la disponibilidad del destino al guardar.

La lista no se recarga automáticamente ni muestra etiquetas de carga que desplacen las tarjetas. Se lee al entrar, después de guardar o eliminar y con el botón de recarga manual. Los resultados mostrados corresponden a esa lectura; el temporizador del servidor continúa funcionando de forma independiente.

En **Administración → Más opciones → Reconexión automática**, agrega destinos AllStarLink, privados o EchoLink. Usa el identificador numérico de app_rpt: para EchoLink, prefijo `3` seguido del número EchoLink (`123456` se introduce como `3123456`). No se transforma automáticamente un número sin prefijo ni se distingue el protocolo por la longitud; app_rpt resuelve el destino igual que en una conexión manual.

Cada entrada permite editar:

- **Nodo fuente:** combo obtenido del descubrimiento de servidores AllStarLink, sin consultas AMI. Incluye los nodos locales, privados marcados como servidores y remotos de Allmon con seguimiento habilitado. Las entradas nuevas preseleccionan el número del principal cuando está disponible; **Principal actual** conserva la selección dinámica de las entradas anteriores.
- **Activada:** control individual, desactivado por defecto. El botón de activación del listado guarda inmediatamente ese cambio. Al desactivarla, la tarjeta muestra **Temporizador detenido**, sin próxima comprobación ni diagnóstico de intentos anteriores. La última fecha, si existe, es histórica. Actualizar la lista no comprueba el enlace: solo lee la configuración y descubre servidores sin consultas AMI.
- **Comprobación cada (segundos):** entero entre `1` y `86400`, inicialmente `60`.
- **Modo:** transceptor, monitor o monitor local, con casilla de permanencia independiente. Los códigos `ilink` son `3/13`, `2/12` y `8/18`, respectivamente para temporal/permanente.

El disquete del formulario guarda las opciones; la papelera pide confirmación antes de eliminar. Desactivar o eliminar solo detiene futuras solicitudes del temporizador: no envía una desconexión ni modifica un enlace permanente de Asterisk. Las entradas no activadas no ejecutan consultas AMI. El guardado y borrado pueden devolver un aviso de operación en curso si coinciden con una comprobación; reintenta sin perder tus cambios.

El programador funciona en el proceso Go, aunque cierres la web o la sesión de administrador. Revisa cada segundo qué entradas están vencidas y comprueba cada una con su frecuencia. La siguiente comprobación se programa al terminar la anterior; las consultas AMI, otras entradas y operaciones concurrentes pueden retrasarla. No es un cron ni una hora exacta. La lista y los tiempos se conservan en SQLite y se retoman al reiniciar Amerimon; no funciona mientras el servicio está detenido. Se reserva el siguiente intento antes de enviar comandos para evitar repeticiones inmediatas tras un fallo.

Antes de cada intento busca la fuente en el mismo inventario que AllStarLink y usa sus credenciales AMI: globales para nodos locales y propias de Allmon para servidores remotos. **Principal actual** resuelve de nuevo el principal en `rpt.conf`; un número fuente explícito no cambia al renombrar el principal. Si la fuente desaparece, tiene seguimiento desactivado o AMI incompleta, registra el error sin conectar desde otro servidor. Puede desactivarse o eliminarse una entrada cuya fuente ya no está disponible. Si `XStat` no contiene una lista interpretable, tampoco conecta. Compara los IDs completos, no subcadenas: `50002` no coincide con `500020`. Un enlace `Conn:` presente en la fuente, incluso conectándose o con otra modalidad, se deja intacto. Los canales SIP no se consideran enlaces RPT ni impiden comprobar los destinos.

Si el destino está ausente, reutiliza la protección de autoenlaces, rutas visibles y reservas pendientes de las conexiones manuales, sin la opción de cambiar modalidad ni desconectar antes. Un intento concurrente se aplaza. Se solicita `rpt cmd <fuente> ilink <código> <destino>` y se registra la operación. La reserva pendiente dura 15 segundos. **Conexión solicitada** solo indica que AMI aceptó el comando; no demuestra conexión ni audio. En el siguiente ciclo se comprueba el estado real. Un fallo AMI o rechazo de topología queda visible con su diagnóstico y se reintenta según la frecuencia. La ausencia de bucles no puede garantizarse para rutas ocultas o nodos inaccesibles.

La tarjeta muestra **fuente ← destino**: el destino es el nodo que se conecta al fuente; la flecha no representa el sentido del audio. Agrupa los iconos de activación, edición y borrado, y separa modo, frecuencia y estado con una insignia de activación. Las fechas y el resultado aparecen debajo; el detalle técnico es desplegable. Se adapta a pantallas estrechas. No sobrescribe el formulario ni sustituye una fuente guardada ausente. Los textos están en español e inglés. Los detalles de Asterisk y del servidor se muestran en su idioma original. Revisa también **Registros Asterisk** y `journalctl -u amerimon` para los intentos.

La migración `0016_node_reconnections.sql` crea `node_reconnections`; `0017_node_reconnection_sources.sql` añade la fuente y unicidad por pareja fuente/destino mediante copia transaccional. Conserva IDs, activaciones, intervalos, modos, fechas, resultados y detalles existentes, asignando `source_node = ""` como alias de **Principal actual**. Se conserva el respaldo previo a migrar bases existentes; no se reescriben migraciones anteriores, YAML, `rpt.conf`, Allmon ni configuraciones del cliente. Una instalación nueva comienza con la lista vacía.

Todas las rutas requieren JWT y rol administrador:

| Endpoint | Uso |
|---|---|
| `GET /mon/node-reconnections` | Devuelve `{ "entries": [...] }` con configuración y estados |
| `POST /mon/node-reconnections` | Crea una entrada |
| `PUT /mon/node-reconnections/:id` | Edita solo esa entrada y reinicia su programación |
| `DELETE /mon/node-reconnections/:id` | Elimina solo esa entrada, sin desconectar |

El cuerpo de creación/edición contiene `source_node` (ID de servidor, o vacío para **Principal actual**), `node`, `enabled`, `interval_seconds`, `action` (`connect`, `monitor`, `localmonitor`) y `permanent`. Cada respuesta de listado añade `id`, `last_check_at`, `next_check_at` (segundos Unix; `0` si aún no hay fecha), `last_result` (clave traducible) y `last_details`. El mismo destino se permite desde fuentes diferentes, sujeto a la protección de topología. Se rechazan parejas duplicadas o fuente igual a destino con HTTP `409`; parámetros inválidos con `400`, IDs inexistentes con `404` y errores de configuración o almacenamiento con `503/500`.

## 6. Administración y funciones de la aplicación

En el primer acceso, pulsa **Admin** y define el usuario y contraseña. El nombre debe tener entre 3 y 32 caracteres permitidos por la aplicación; la contraseña requiere al menos 8 caracteres y se guarda con bcrypt. Los endpoints administrativos requieren JWT de rol administrador.

### Pantalla Nextion

En **Administración → Pantalla Nextion**, configura el dispositivo serie USB, la velocidad, la interfaz de red y el nodo favorito. El estado de activación, el dispositivo, la velocidad, el favorito y el envío de etiquetas internacionalizadas se guardan en SQLite; el nodo local y las credenciales AMI se reutilizan desde la configuración de Amerimon. Se muestran las interfaces activas distintas de loopback, con IPv4 y MAC; en modo automático Amerimon elige una IPv4 disponible. El favorito se selecciona entre los nodos de Favoritos. Se pueden guardar valores incompletos mientras el controlador esté desactivado; al activarlo se validan todos los campos necesarios. El botón de prueba abre el dispositivo serie USB seleccionado con la velocidad elegida y lo cierra inmediatamente; no envía comandos a la pantalla ni la ocupa de manera permanente. GPIO/UART solo corresponde a instalaciones con hardware compatible.

Los dispositivos serie USB se enumeran como `/dev/ttyUSB*` o `/dev/ttyACM*`. El identificador USB `10c4:ea60` corresponde a un adaptador Silicon Labs CP210x. El nodo local se detecta desde la configuración RPT de Asterisk; la IPv4 actual también se muestra debajo del selector de interfaz.

El firmware es único para todos los idiomas: Amerimon envía los textos de botones y títulos (`b_fav.txt`, `b_disc_all.txt`, `t_hint.txt`, `b_restart.txt`, `b_reboot.txt`, `b_poweroff.txt`, `t_actions.txt`, `t_links.txt`, `b_admin_no.txt`, `b_admin_ok.txt`) y la variable `va_lang.val` (`0` español, `1` inglés) según el idioma elegido con el botón de idioma de la cabecera de la web; la pantalla usa `va_lang` solo para sus textos internos de respaldo cuando pierde la conexión. Los mensajes dinámicos también se envían en ese idioma. La opción **Enviar etiquetas internacionalizadas a la pantalla** está activada de forma predeterminada; desactívala si usas un diseño HMI propio y quieres conservar tus etiquetas: solo se enviarán los valores.

Todos los componentes referenciados por código deben tener `vscope=global`. En Nextion, `txt` significa texto y `bco`, color de fondo. El controlador envía estos valores:

| Componente | Contenido |
| --- | --- |
| `t_nodo.txt` | Número del nodo local con el prefijo `Nodo:` o `Node:` según el idioma. |
| `t_ip.txt` | Dirección IPv4 seleccionada para la interfaz. |
| `t_date.txt`, `t_time.txt` | Fecha local `YYYY-MM-DD` y hora local de 24 horas `HH:mm:ss`. |
| `t_temp.txt` | Temperatura de CPU en °C; `--` si no se puede leer. |
| `t_volt.txt`, `t_cpu.txt`, `t_mem.txt`, `t_dsk.txt` | Voltaje del sistema y porcentajes de CPU, memoria y disco; `--` si no se pueden leer. |
| `t_links.txt` | Título `Enlaces activos` o `Active links` según el idioma; añade la cantidad cuando hay más de ocho enlaces. |
| `t_actions.txt`, `t_hint.txt` | Título de acciones y ayuda táctil, enviados según el idioma. |
| `b_fav.txt`, `b_disc_all.txt`, `b_restart.txt`, `b_reboot.txt`, `b_poweroff.txt`, `b_admin_no.txt`, `b_admin_ok.txt` | Textos de los botones, enviados según el idioma. |
| `t_n1.txt`–`t_n8.txt` | Hasta ocho enlaces activos: IAX muestra el número; IAXRpt, el indicativo; EchoLink, el indicativo disponible o el número. Estos tres no llevan prefijos. SIP conserva `SIP` seguido del caller ID o endpoint. Una fila vacía muestra `--`. |
| `t0.txt` | Indicativo o número del origen del audio junto a su descripción disponible en AstDB o el catálogo privado, por ejemplo `700001 - Remote station`. Sin descripción muestra solo el identificador. No añade ubicación ni textos como «Transmitiendo» o «Recibiendo». SIP conserva solo el caller ID o endpoint, sin `SIP`. Varias fuentes activas se separan por comas; las filas de enlaces no cambian. |
| `b_estado.bco`, `t0.bco` | Verde `1024`: recepción; rojo `63488`: transmisión sin recepción; gris `33840`: sin actividad. |
| `b_d1`–`b_d8` | Botones transparentes sobre cada fila; solo son visibles cuando hay un enlace. |
| `va_alive.val` | Latido que Amerimon actualiza a `4` una vez por segundo. |
| `va_lang.val` | Idioma activo: `0` español, `1` inglés. |

La recepción se detecta por COS local (`SignalOnInput`), RX activo de enlaces IAX/IAXRpt/EchoLink (`SawStat`) y eventos de voz SIP del canal conectado. Un canal SIP conectado o su PTT activo, sin voz detectada, no basta para indicar recepción. La recepción tiene prioridad sobre TX: si Asterisk activa el transmisor para retransmitir audio recibido, la pantalla permanece verde y muestra quien origina ese audio. Para COS local o TX sin recepción, se muestra el número y la descripción disponible del nodo local, no un indicativo de persona que Asterisk no ha identificado.

Al terminar la actividad, `t0.txt` queda vacío y el color pasa a gris, salvo que continúe TX u otra fuente activa. Los mensajes temporales de operaciones del sistema y los avisos de desconexión del controlador mantienen su prioridad sobre el texto de audio.

Configura cada acción en **Touch Release Event** con la cadena indicada, sin comillas ni terminador adicional:

| Acción | Cadena | Resultado |
| --- | --- | --- |
| Conectar favorito | `CONN_FAV` | Conecta el favorito configurado si aún no está conectado. |
| Desconectar todos | `DISC_ALL` | Ejecuta el equivalente a `*70` (`ilink 6`) en el nodo local. |
| Mantener pulsada una fila | `HOLD_N<número>` + espacio + texto de la fila + LF | Registra la conexión real asociada a la fila. |
| Soltar una fila | `DISC_N<número>` | Desconecta el enlace registrado si todavía está activo. |
| Preparar reinicio de Asterisk / Linux o apagado de Linux | `SYS_ARM_RESTART`, `SYS_ARM_REBOOT` o `SYS_ARM_OFF` | Arma la acción correspondiente durante 10 segundos; requiere confirmación. |
| Confirmar o cancelar acción del sistema | `SYS_CONFIRM` / `SYS_CANCEL` | Ejecuta la acción armada vigente o la cancela. |

`DISC_N1`–`DISC_N8` por sí solas no bastan para desconectar: primero debe recibirse el evento de pulsación larga de la fila. El controlador conserva el identificador real de la conexión y no usa el indicativo mostrado como sustituto del número EchoLink o del canal SIP. SIP se desconecta por su canal PJSIP; IAX, IAXRpt y EchoLink mediante el enlace RPT. La respuesta estándar `sendme` de Nextion reinicia el estado de interfaz al reconectarse la pantalla.

Los archivos de cada modelo son únicos para todos los idiomas y comparten este nombre base:

- `public/assets/firmware/AMERINET_Nextion_<modelo>.HMI`: fuente editable.
- `public/assets/firmware/AMERINET_Nextion_<modelo>.tft`: firmware para instalar desde tarjeta SD.
- `public/assets/images/AMERINET_Nextion_<modelo>.png`: vista previa.

Por ejemplo, `AMERINET_Nextion_NX4832K035` identifica los archivos del modelo NX4832K035. Los nombres antiguos con sufijo de idioma (`-es`, `-en`) ya no se reconocen. El catálogo administrativo agrupa los archivos, muestra la vista previa y ofrece descargas independientes de TFT y, si existe, HMI. Copia el TFT a una tarjeta SD para actualizar la pantalla; otros modelos siguen el mismo formato de nombre. Los archivos se instalan en `/usr/share/amerimon/nextion/`; el directorio se sirve mediante la ruta administrativa autenticada y no se incluye en los assets públicos de Angular.

### Configuración de EchoLink

El control del módulo y su estado utilizan el CLI local (`asterisk -rx`), no la acción AMI `Command`, que bloquea `module load` y `module unload`. El servicio requiere acceso al ejecutable y al socket de control de Asterisk. Solo ejecuta comandos fijos de EchoLink, con tiempo límite; no acepta comandos arbitrarios del navegador.

Al entrar, el estado actual de `chan_echolink.so` se destaca en un recuadro verde si está cargado o rojo si está descargado, con texto explícito. Indica el estado del módulo, no una conexión activa con otro cliente. Se actualiza al confirmar una activación/desactivación o pulsar **Actualizar**.

**Importante: las conexiones EchoLink entrantes también están sujetas a las listas de acceso del nodo.** Con `context=radio-secure`, pasan por el mismo contexto que las conexiones AllStarLink. Si `allowlist/<nodo-destino>` contiene entradas, solo se permiten los números incluidos; si está vacía, se permiten salvo los incluidos en `denylist/<nodo-destino>`. Una coincidencia en allowlist tiene prioridad sobre denylist. Para EchoLink, utiliza el número de conexión con prefijo `3`: el cliente EchoLink `626066` se registra como `3626066` en las listas del nodo destino. Un cliente puede conectar hacia afuera y ser rechazado al intentar entrar, porque la entrada sí evalúa estas reglas. El patch de `peerip` no elimina ni evita este control.

El postinstall aplica `echolink-peerip.awk` únicamente a las consultas estándar de `peerip` de `[radio-secure]`: IAX2/SIP consultan la IP, mientras EchoLink continúa sin consultar esa propiedad ni recibir una IP local ficticia. Conserva la lógica de listas existente y los demás contextos. Si cambia el archivo, crea `.bak-echolink-peerip`, conserva propietario/permisos y utiliza la recarga de dialplan del postinstall, sin reiniciar Asterisk. La aplicación es idempotente y rechaza secciones duplicadas o modificaciones parciales. No garantiza por sí sola una conexión EchoLink entrante.

La activación permanente admite asignaciones `load =` y `load =>` en `modules.conf`. Cambia la misma línea de EchoLink entre `load` y `noload`, conservando formato, comentario y posición. Si hay duplicados de EchoLink, conserva únicamente esa línea; solo añade una entrada si no existe ninguna. Los demás módulos no cambian.

En **Radio y ubicación**, **Obtener coordenadas del navegador** solicita permiso y rellena latitud y longitud con la ubicación del dispositivo del administrador, no del servidor. Requiere HTTPS o localhost. Comprueba que corresponde a la estación; puedes corregir los valores manualmente. No guarda ni aplica automáticamente: pulsa **Guardar** después de revisarlos. Si se deniega el permiso o falla la consulta, se muestra el error y se conservan los valores anteriores.

Las directivas opcionales `#tryinclude` que apuntan a un archivo ausente, sin comodines, no bloquean el formulario y se conservan al guardar. Si el archivo incluido existe o no se puede verificar su ausencia, la edición se bloquea para no ignorar opciones que Asterisk podría estar aplicando.

En **Más opciones → Configuración de EchoLink** (`/admin/echolink`), un administrador puede editar las secciones `elN` existentes de `echolink.conf`. Los archivos se buscan en el mismo directorio que el `rpt.conf` configurado (normalmente `/etc/asterisk`). El campo **Mensaje de conexión** ocupa una fila completa de la cuadrícula del formulario.

1. Selecciona la sección y revisa identidad, radio y servidores. `node` es el número EchoLink; `astnode` se guarda con el número del nodo principal actual. La contraseña no se incluye en la consulta inicial: pulsa el ojo con el campo vacío para consultar la guardada, o escribe una nueva y usa el ojo para verla u ocultarla. La consulta requiere acceso de administrador, verifica la revisión del archivo y no permite caché; utiliza HTTPS. Dejar el campo vacío al guardar conserva la contraseña existente.
2. **Guardar** crea un respaldo `.amerimon.bak` con permisos `0600` y actualiza únicamente la sección seleccionada, conservando opciones desconocidas y otras secciones. Un cambio externo del archivo o del nodo principal exige recargar antes de guardar. Los includes y las secciones ambiguas requieren edición manual.
3. **Activar** ejecuta `module load chan_echolink.so`; **Desactivar**, `module unload -f chan_echolink.so`, tanto en modo temporal como permanente, tras confirmación. La descarga forzada corta conexiones y puede afectar la estabilidad de Asterisk si el módulo está en uso; la confirmación advierte este riesgo. Se verifica después que el módulo ya no esté cargado. Si se marca **Cambio permanente**, además se cambia la misma línea a `noload` en `[modules]` de `modules.conf`. Sin marcarlo, el archivo no cambia. No se reinicia Asterisk.
4. Guardar no aplica automáticamente la configuración. Para un módulo ya activo, solicita **Aplicar configuración: recargar módulo** y confirma. Si Asterisk no admite la recarga, rechaza la descarga o no confirma el estado esperado, se muestra el error. Un cambio permanente ya guardado permanece aunque falle la operación del módulo: revisa el archivo y los registros antes de reintentar.
5. **Verificar puertos** prueba conexiones TCP salientes a los servidores configurados en el puerto **5200** y comprueba sockets locales UDP **5198 y 5199**. No demuestra acceso desde Internet ni identifica al propietario de esos sockets.

**La visibilidad de los puertos en Internet no es responsabilidad de Amerimon.** Router, NAT, firewall y restricciones del proveedor corresponden al administrador. Amerimon no abre puertos ni cambia reglas de red. Estas comprobaciones tampoco sustituyen una prueba de conexión y audio con un cliente EchoLink externo. La configuración del modo de telemetría para emitir audios sigue siendo independiente (véase la sección de audio).

### Nodos privados: catálogo y configuración de Asterisk

**Admin → Nodos privados** contiene dos editores en la misma pantalla:

- **Catálogo:** líneas de `privatenodes.txt` con formato `nodo|indicativo|descripción|ubicación`. Mantiene los datos descriptivos para AstDB.
- **Nodos privados en rpt.conf:** listado de secciones y entradas de cuatro dígitos del archivo indicado por `ami.rpt_config_path` (por defecto `/etc/asterisk/rpt.conf`). Permite crear, editar en una tarjeta emergente y eliminar con confirmación.

Los guardados son independientes. Crear un nodo en `rpt.conf` no agrega una línea al catálogo; borrar una línea del catálogo tampoco elimina la sección de Asterisk. Mantén ambos registros desde sus respectivos editores cuando necesites descripción y configuración del mismo nodo. Los nodos privados no se convierten en el principal; por defecto no generan tablas de servidores del monitor.

Cada fila del listado tiene **Mostrar como servidor en AllStarLink**, desactivado por defecto. Marca o desmarca los nodos y pulsa **Guardar cambios**, como en el listado Allmon. El guardado se habilita solo con cambios y conserva la selección si falla la solicitud. Un nodo marcado obtiene una tabla propia y aparece en el selector de servidores locales, con las credenciales AMI globales; su categoría privada y la elección del principal no cambian. Se aplica en la próxima actualización del monitor tanto con Allmon como sin él. Desmarcarlo no desconecta sus enlaces: solo retira su tabla y opción de servidor.

La tarjeta separa el número y `rxchannel` de la casilla y su ayuda. Los botones son iconos: disquete para **Guardar cambios**, edición para **Editar** y papelera para **Eliminar**. Incluyen ayuda al pasar el puntero y etiquetas accesibles traducidas; el disquete muestra un indicador de actividad mientras guarda.

La preferencia se guarda como comentario `; amerimon_show_as_server = yes` dentro de la sección del nodo en `rpt.conf`, no como parámetro de Asterisk ni en la plantilla. Se retira al desmarcar; eliminar el nodo también elimina la preferencia. La API incluye el booleano `show_as_server` en cada nodo de `GET/PUT /mon/private-nodes/rpt`.


`sources.privatenodes_path` usa `/var/lib/asterisk/privatenodes.txt` de forma predeterminada. Al iniciar, Amerimon conserva el archivo ya existente en la ruta configurada; si está en otra ubicación y el predeterminado aún no existe, crea un enlace simbólico hacia él. Si se usa la ruta predeterminada y falta o está vacía, busca primero archivos con contenido en `/etc/allmon3/privatenodes.txt` y `/var/www/html/supermon/privatenodes.txt`, enlazándolos para mostrar y editar los nodos existentes sin duplicarlos. Nunca reemplaza un archivo con datos ni un enlace existente. Si no hay ningún archivo previo, crea el predeterminado. El administrador puede editar, agregar y borrar líneas desde **Nodos privados**; al guardar se conservan los permisos y el grupo del archivo compartido. El servicio instalado, ejecutado como root, conserva además su propietario; en una instalación personalizada sin privilegios, el reemplazo puede pasar al usuario del proceso y se registra esta situación en el log. Cada registro usa `nodo|indicativo|descripción|ubicación`.

#### Crear o editar un nodo

1. Selecciona **Agregar nodo privado** o **Editar** en el listado de `rpt.conf`.
2. Para un nodo nuevo introduce un número de exactamente cuatro dígitos. El número no se cambia al editar un nodo existente.
3. Define el `rxchannel` y modifica los parámetros que corresponda. El formulario nuevo precarga los valores del ejemplo 1998:

   | Parámetro | Valor inicial |
   | --- | --- |
   | `rxchannel` | `USRP/127.0.0.1:34001:32001` |
   | `duplex` | `0` |
   | `hangtime` | `0` |
   | `althangtime` | `0` |
   | `holdofftelem` | `1` |
   | `telemdefault` | `0` |
   | `telemdynamic` | `0` |
   | `linktolink` | `no` |
   | `nounkeyct` | `1` |
   | `totime` | `180000` |

   Estos valores son editables, no una configuración universal para todo hardware. No se activa el `startup_macro` comentado del ejemplo.
4. El combo **Parámetro** recoge las asignaciones activas de `[node-main]` que no están ya configuradas en el nodo; `rxchannel` tiene su propio campo y no se ofrece otra vez. Al seleccionar, se precarga el valor de la plantilla y se muestran los comentarios inmediatamente anteriores y el comentario de la misma línea, si existen. **Agregar sobrescritura** añade el valor a la sección del nodo, sin cambiar la plantilla.
5. **Escribir otro parámetro…** permite introducir un nombre personalizado formado por letras, números o guion bajo y su valor. No admite parámetros repetidos. Los parámetros ya presentes se editan en sus campos.
6. **Guardar cambios** se habilita al modificar el formulario; volver a los valores originales lo deshabilita. **Cancelar** descarta la edición de la tarjeta sin escribir archivos.

La interfaz está traducida al español y al inglés. La documentación recogida del archivo se muestra como texto en su idioma original, sin traducción automática; si no hay comentarios, se indica que falta la descripción. La extracción del combo se limita a las asignaciones activas de `[node-main]` en el archivo principal: no incluye parámetros comentados ni resuelve archivos incluidos o plantillas heredadas.

#### Ayuda y validación de rxchannel

Los ejemplos aparecen dentro de la tarjeta, junto al campo:

| Canal de ejemplo | Uso |
| --- | --- |
| `Local/pseudo` | Sin radio (hub) |
| `SimpleUSB/1999` | SimpleUSB |
| `Radio/1999` | USBRadio (DSP) |
| `Voter/1999` | Dispositivo RTCM |
| `Pi/1` | Raspberry Pi PiTA |
| `Dahdi/1` | Tarjeta PCI Quad |
| `Beagle/1` | BeagleBoard |
| `USRP/127.0.0.1:34001:32001` | Interfaz GNU Radio USRP |

Adapta el dispositivo y los puertos al servidor. El controlador elegido debe estar disponible y habilitado en Asterisk; este editor no modifica `modules.conf`. No se permiten números o `rxchannel` repetidos. También se rechaza el mismo par de puertos USRP, incluso si cambia el host, y se comparan los canales definidos directamente en secciones de nodos públicos. La comprobación no resuelve canales heredados o incluidos y no prueba la disponibilidad física del dispositivo.

Los nodos nuevos se agregan como `[<nodo>](node-main)` y reciben la entrada `<nodo> = radio@127.0.0.1/<nodo>,NONE` en `[nodes]`. Las entradas, cabeceras, comentarios y parámetros existentes se conservan al editar; solo se sobrescriben los valores enviados y se agregan los que faltan. Omitir un parámetro no lo borra. **Eliminar** quita la sección completa del nodo, incluidas sus personalizaciones, y su entrada en `[nodes]`; no elimina archivos de otros controladores ni referencias externas.

Antes de guardar se comprueba que los números de los nodos privados no coincidan con ningún ID de sección PJSIP (endpoints, auth, AOR o plantillas). Se consulta `ami.pjsip_config_path`, o `pjsip.conf` en el directorio de `rpt.conf` si no está configurado. Se recorren `#include` y `#tryinclude`, incluidos comodines; las rutas relativas se resuelven desde el directorio de la configuración principal PJSIP. Un conflicto devuelve HTTP 409 sin escribir, respaldar ni recargar. Errores de lectura, includes obligatorios ausentes, ciclos o directivas no admitidas como `#exec` bloquean el guardado con un error de verificación; un `#tryinclude` sin archivos es válido. Esta comprobación se ejecuta al guardar los nodos privados, no al crear extensiones desde el editor PJSIP.

#### Guardado, respaldo y recarga

Antes de modificar `rpt.conf`, se crea `rpt.conf.amerimon.bak` (o el nombre configurado seguido de `.amerimon.bak`) con permisos `0600`. El respaldo se actualiza con el contenido anterior en cada guardado que cambia el archivo. La escritura conserva permisos, propietario y grupo del destino y respeta los enlaces simbólicos; el servicio necesita permisos para leer, respaldar y reemplazar el archivo.

Tras guardar o eliminar se ejecuta por AMI `module reload app_rpt.so`. Amerimon exige una respuesta de recarga satisfactoria y consulta después `module show like app_rpt.so`; informa éxito solo si el módulo aparece cargado. No reinicia Asterisk. Si la recarga o su verificación fallan, se muestra un error y la configuración puede haber quedado guardada: revisa los registros y el archivo antes de reintentar. No hay restauración automática por un fallo de AMI.

Para un diagnóstico manual en el servidor:

```bash
sudo asterisk -rx 'module reload app_rpt.so'
sudo asterisk -rx 'module show like app_rpt.so'
```

Que el módulo esté cargado no demuestra que cada nodo haya adoptado todos sus valores ni que tenga audio; comprueba también el funcionamiento de las conexiones y del canal elegido.

La API requiere JWT con rol administrador:

| Método y ruta | Contenido |
| --- | --- |
| `GET /mon/private-nodes` | Ruta, permisos y líneas del catálogo |
| `PUT /mon/private-nodes` | Guarda `{ "lines": [...] }` |
| `GET /mon/private-nodes/rpt` | `path`, `nodes`, `template_settings` y `template_descriptions` |
| `PUT /mon/private-nodes/rpt` | Guarda `{ "nodes": [...] }`; cada nodo tiene `id`, `rxchannel` y `settings` |

El PUT de `rpt.conf` envía el conjunto completo de nodos privados: los ausentes se eliminan. Evita editar simultáneamente desde varios navegadores; recarga la lista antes de trabajar con una configuración modificada fuera del editor. Una respuesta satisfactoria incluye `reloaded: true` y `module_loaded: true`.

### Control de acceso: nodos e indicativos

Los contextos del dialplan tienen funciones diferentes:

- **`[radio-secure]`:** controla conexiones entre nodos. Consulta `allowlist/<destino>/<nodo-origen>` y `denylist/<destino>/<nodo-origen>`, usando `${EXTEN}` como destino y `${CALLERID(num)}` como nodo de origen. El patch de `allstar-public` no modifica este contexto.
- **`[allstar-public]`:** atiende clientes como IAX Web Transceiver, Repeater Phone y QSO One. Primero autentica el indicativo mediante AllStarLink; una autenticación inválida termina la llamada. Aquí `${CALLSIGN}` es el indicativo autenticado y `${NODENUM}`, obtenido de `${CALLERID(num)}`, es el nodo de destino, no un nodo de origen.

Después de autenticar, `allstar-public` aplica este orden:

Para las comparaciones obtiene `CALLSIGN_BASE`: elimina todo desde el primer `-` o `/` y convierte a mayúsculas. Por ejemplo, `HC1LNM-P`, `HC1LNM/RPT` y `HC1LNM-P/RPT` se comparan como `HC1LNM`. Guarda las entradas de acceso con el indicativo puro, sin sufijos. El indicativo completo autenticado se conserva en la identidad de la conexión y en los mensajes del dialplan; una base vacía se rechaza. Esta normalización no afecta a los números de nodo ni a `[radio-secure]`.

1. Si existe `allowlist/${NODENUM}/${CALLSIGN_BASE}`, conecta directamente.
2. Si no existe esa entrada, consulta `denylist/${NODENUM}/${CALLSIGN_BASE}`. Si existe, bloquea la llamada.
3. Si el indicativo no aparece en ninguna de esas dos listas del destino, pasa al filtro global `whitelist/${CALLSIGN_BASE}` y `blacklist/${CALLSIGN_BASE}`. Tener otras entradas en `allowlist/<destino>` no impide este respaldo.

Las decisiones del primer filtro son definitivas: una coincidencia en `allowlist` tiene prioridad sobre `denylist` y no consulta las listas globales; una coincidencia en `denylist` tampoco puede ser anulada por la `whitelist` global.

El filtro global conserva la política del patch: con ambas listas vacías permite el acceso; una coincidencia en `whitelist` permite el acceso incluso si también está en `blacklist`; sin coincidencia en `whitelist`, una coincidencia en `blacklist` bloquea. Si al menos una lista global tiene entradas y el indicativo no coincide en ninguna, también bloquea, incluso cuando solo está poblada `blacklist`.

En **Admin → Nodo principal**, las tarjetas separan las listas del nodo (`allowlist/denylist`) de las listas globales por indicativo (`whitelist/blacklist`). Las entradas globales no llevan número de nodo y sus cambios mediante AMI se aplican inmediatamente. La instalación prepara las comprobaciones en la stanza existente `[allstar-public]` y recarga el dialplan, sin modificar `[radio-secure]`.

### Configuración del nodo principal

En **Admin → Nodo principal** están disponibles las funciones equivalentes a las opciones **1 Node number**, **2 Node Password**, **3 Node callsign** y **7 Node access list** de `node-setup`.

- **Número:** cambia el número público del nodo principal identificado en `rpt.conf`. Este monitor exige un nodo principal público (4 a 6 dígitos sin comenzar por 0 o 1); los nodos privados se administran como conexiones, no como principal. Se rechazan números ya configurados y formularios abiertos para un principal que haya cambiado.
- **Contraseña:** es la contraseña de registro obtenida del portal AllStarLink, no la de Amerimon ni la de AMI. Solo el administrador puede recuperar la contraseña actual, que aparece oculta y puede mostrarse con el botón del ojo. Se prioriza el registro HTTP activo y, si falta, el registro IAX. La respuesta no se almacena en caché; la contraseña no se guarda en el almacenamiento del navegador ni en los registros. Utiliza HTTPS y un dispositivo de confianza. Dejar el campo sin cambios o vacío conserva la actual; una nueva contraseña admite entre 6 y 128 letras, números, guion y guion bajo.
- **Indicativo:** actualiza `idrecording = |iINDICATIVO` directamente en la sección principal, conservando las plantillas. Un `idrecording` heredado o basado en un archivo de audio no se presenta como un indicativo editable: introduce el indicativo que quieras utilizar.
- **Dúplex:** permite configurar `duplex` (0–4) directamente en la sección del nodo principal; si el valor se heredaba de una plantilla, se agrega una anulación local sin modificar la plantilla. Las opciones son: 0, semidúplex sin telemetría ni hang time (full duplex con `linktolink=yes`; recomendado para controlador externo multiport; comentar `idrecording` e `idtalkover` también suprime IDs); 1, semidúplex con telemetría y hang time, sin repetir audio (recomendado para nodo simplex); 2, full duplex con telemetría y hang time (recomendado para repetidor); 3, full duplex con telemetría y hang time, sin repetir audio; 4, full duplex con telemetría y hang time, repite audio solo con autopatch inactivo. Para emitir audio global a nodos enlazados, `duplex` debe ser distinto de 0 (modos 1–4).
- **Acceso:** permite agregar, actualizar y eliminar números o indicativos en `allowlist/<principal>` y `denylist/<principal>` de AstDB, con un motivo opcional. Los cambios se aplican inmediatamente mediante AMI. Sin entradas no hay restricciones de estas listas. Una lista de permitidos limita los nodos admitidos; los bloqueados se rechazan. Los motivos admiten hasta 200 caracteres, sin comillas dobles, barras inversas ni caracteres de control.
- **Acceso AMI:** antes de las listas de acceso se puede cambiar el `secret` del usuario AMI ya configurado en `ami.user`, la dirección `bindaddr` de `[general]` y, opcionalmente, añadir redes `permit`/`deny` a esa misma stanza. El secret aparece enmascarado y puede mostrarse con el botón del ojo. Las redes IPv4 aceptan IP/prefijo o IP/máscara; Amerimon calcula la red normalizada. Las reglas existentes son de solo conservación: la pantalla agrega, pero no elimina ni reemplaza opciones personalizadas. Se rechaza cualquier `deny` que abarque loopback o la dirección del backend y se agrega permiso explícito de loopback al restringir con `permit`. Si no se puede resolver la dirección AMI del backend, no se aplican cambios de ACL.
- Esta edición crea respaldos `.amerimon.bak` de `manager.conf`, `amerimon.yaml` y `amerimon.env`; actualiza `ami.host`, `ami.user` y la referencia segura `ami.secret: ${AMI_SECRET}` en el YAML, y mantiene el valor secreto en el archivo de entorno con permisos existentes. El servicio instalado indica explícitamente la ruta `/etc/amerimon/amerimon.env`; en instalaciones personalizadas, define `AMERIMON_ENV_FILE` con la ruta del archivo de entorno usado por el proceso. Al usar un `bindaddr` concreto, el host AMI del YAML se sincroniza con esa dirección; con `0.0.0.0` o `::` se conserva el host actual. Luego ejecuta `manager reload` (sin reiniciar Asterisk) y comprueba una nueva conexión AMI. Si la autenticación falla, intenta restaurar los archivos y recargar la configuración previa. No reinicies Asterisk si se informa un fallo de restauración.

La edición modifica únicamente referencias al nodo seleccionado en `rpt.conf`, `rpt_http_registrations.conf`, `iax.conf` y, si existen, `savenode.conf`, `extensions.conf`, `simpleusb.conf`, `usbradio.conf` y `voter.conf`. También renombra la sección correspondiente en el Allmon activo y en `/etc/allmon3/allmon3.ini`, cuando existen. Las demás secciones, registros y credenciales se conservan. Al renumerar, las listas de acceso se trasladan al nuevo número; se rechaza un destino que ya tenga listas.

Las definiciones editadas deben estar directamente en esos archivos: no se reescriben archivos `#include`, personalizaciones arbitrarias ni referencias numéricas en scripts externos. Si falta la sección principal directa, el guardado se rechaza. El proceso debe poder leer los archivos, crear respaldos y reemplazarlos en sus directorios, conservando propietario, grupo y permisos. No se elevan privilegios desde la web. Las rutas de Asterisk se derivan del directorio de `ami.rpt_config_path`.

Antes de modificar archivos se generan respaldos adyacentes `*.amerimon.bak` con permisos `0600`; contienen contraseñas y no deben publicarse ni copiarse a ubicaciones accesibles por HTTP. Se conserva el último respaldo de cada archivo. Si falla el guardado o la migración de listas, se intenta restaurar la configuración anterior y se informa explícitamente si alguna restauración falla. No reinicies Asterisk si se ha notificado un fallo de restauración: revisa los respaldos primero.

**Guardar recarga `app_rpt.so` automáticamente** mediante AMI con `module reload app_rpt.so`; no reinicia Asterisk. La pantalla confirma el guardado solo si AMI acepta la recarga. Si la recarga falla, los archivos ya están guardados con sus respaldos `.amerimon.bak`; revisa los registros de Asterisk y no repitas cambios de identidad hasta confirmar qué configuración está activa. Si cambiaste el número y usas Allmon3, reinicia Allmon3 manualmente. Programa los cambios en una ventana de mantenimiento y considera su impacto en la operación del nodo.

Los botones **Vaciar permitidos** y **Vaciar denegados** eliminan exclusivamente las entradas de la lista elegida del nodo principal, previa confirmación, sin tocar la otra lista ni otros nodos. El cambio es inmediato y no tiene deshacer. Vaciar permitidos elimina la restricción a los nodos autorizados; la lista de denegados sigue aplicándose. Si falla una eliminación, se informa que el vaciado puede ser parcial y debe recargarse la lista.

**Exportar listas** descarga ambas listas y sus motivos en `listas-acceso-<nodo>.json`, consultando AstDB de nuevo. El documento usa `format: "amerimon-node-access"`, `version: 1`, `node` (origen) y `entries` con `type`, `node` y `reason`. No incluye contraseñas. **Importar listas** permite seleccionar ese JSON (hasta 1 MiB y 1000 entradas) y confirmar el nodo de destino. Siempre combina: conserva entradas ajenas al archivo y actualiza los motivos de las coincidentes por lista y nodo/indicativo. Puede importar un archivo de otro nodo, pero se aplica únicamente al principal actual. No permite entradas duplicadas en una misma lista ni incluir el propio nodo de destino. Valida el archivo completo antes de escribir, verifica las entradas guardadas e intenta restaurar los cambios si falla una operación, notificando cualquier restauración incompleta. Los cambios son inmediatos. La exportación requiere `GET /mon/primary-node/access/export`; la importación, `POST /mon/primary-node/access/import` con `primary_node` y `document`, ambos restringidos al administrador.

La API requiere JWT con rol administrador para `GET/PUT /mon/primary-node`, `GET/PUT /mon/primary-node/ami`, `GET/PUT/DELETE /mon/primary-node/access` y `POST /mon/primary-node/access/clear`. El vaciado requiere `primary_node`, `type` (`allowlist` o `denylist`) y `confirmation` (`VACIAR allowlist` o `VACIAR denylist`).

### Herramientas públicas de contraseñas

El enlace **Herramientas de contraseñas** del encabezado abre una sola página (`/password-tools`) con el generador y el verificador. Las antiguas rutas `/password-generator` y `/password-meter` redirigen a esa página.

El generador propone inicialmente una contraseña de 12 caracteres y admite un mínimo de 8. Puedes combinar mayúsculas, minúsculas, números y símbolos; al elegir tipos manualmente, selecciona al menos dos. Si no marcas ninguno, se usan los cuatro. La selección aleatoria usa la API criptográfica del navegador y favorece los caracteres menos repetidos para aumentar la diversidad. Puedes regenerar la clave o copiarla al portapapeles. La clave generada se envía al verificador de la misma página.

El verificador también permite introducir una contraseña de hasta 128 caracteres y mostrarla u ocultarla. Presenta puntuación, complejidad, detalle de adiciones y deducciones, entropía de Shannon y recomendaciones sobre longitud, diversidad, grupos de caracteres, repeticiones y patrones predecibles. La longitud mínima indicada es 8; las recomendaciones sugieren al menos 12 caracteres.

La entropía mostrada se calcula a partir de las frecuencias de caracteres observadas en la contraseña. Describe la diversidad de esa muestra; no es una medida de la imprevisibilidad criptográfica del generador ni una garantía de que una clave sea segura. La generación y evaluación se realizan en el navegador; estas herramientas no envían ni guardan la contraseña en el backend de Amerimon.

### Gestión y reproducción de audios

Solo los administradores ven, en la cabecera de la tabla AllStarLink, el icono de **grabación/reproducción** (círculo y triángulo) para abrir **Gestión de audios en otra ventana**. Desde Gestión de audios, el icono de **tabla** abre la tabla AllStarLink. Ambos conservan descripciones emergentes y etiquetas accesibles en español e inglés. La página original permanece abierta para observar conexiones y manejar audios a la vez. Permite ventanas emergentes para el sitio si el navegador las bloquea; puede decidir abrir una pestaña en lugar de una ventana. No se transfieren credenciales: si la sesión no está recordada, puede ser necesario iniciar sesión en la nueva ventana. La ruta de audios conserva su autorización administrativa.

**Administración → Gestión de audios** (`/admin/audio`) es una opción exclusiva del administrador. Permite conservar una biblioteca de audios WAV para activarlos o desactivarlos cuando sea necesario. Los archivos y estados se guardan en el backend y SQLite; cerrar sesión o cerrar la página no detiene la programación. Los audios permanecen hasta que el administrador los elimine explícitamente.

- Formatos de subida: WAV, MP3, FLAC, OGG/Opus, M4A/MP4, AAC, WMA, AIFF, WebM, MKA, AMR y APE, según los códecs de FFmpeg instalado. Ya no se exige subir solo WAV: el servidor convierte a WAV PCM mono de 16 bits y 8000 Hz. Máximo 25 MiB tanto de entrada como convertido; una conversión que alcanza el límite se rechaza sin guardar un audio truncado. Se permite una conversión simultánea por servicio, con un máximo de 60 segundos. Instala `ffmpeg` antes de usar el instalador local; el paquete Debian lo declara como dependencia. Los WAV ya compatibles no necesitan conversión externa.
- `broadcast_once`: el administrador pulsa el icono de emisión **Reproducir en Asterisk**; Asterisk inicia una emisión al nodo principal y sus estaciones enlazadas. Esta acción es puntual y no se repite por el hecho de mantener el audio activado.
- `broadcast_silence`: cada audio activado mantiene su propio intervalo, de 1 a 10080 minutos. Al vencer, se captura la mezcla del nodo principal mediante `amerimon-record-node` y EAGI, igual que en las grabaciones. Se exigen cinco segundos de muestras continuas por debajo de aproximadamente −40 dBFS y se vuelve a comprobar la captura tras preparar el archivo. Si se detecta audio, se espera al siguiente intervalo; si la captura falla o deja de entregar muestras, se registra el error y la emisión permanece pendiente. La reserva persistente actualiza la próxima fecha antes de enviar, en la misma transacción que el bloqueo de reproducción, para impedir que dos ciclos reclamen la misma emisión. Solo se intenta un audio por ciclo. La reserva dura la duración seleccionada más dos segundos; si Asterisk rechaza la emisión, se registra el error y se espera al siguiente intervalo.
- Para distribuir el audio desde el nodo principal hacia los nodos enlazados, `duplex` debe ser distinto de 0 (modos 1–4); Amerimon valida el valor efectivo, incluso si se hereda de una plantilla. La pantalla de configuración del nodo principal permite cambiar `duplex` y ejecuta automáticamente `module reload app_rpt.so` al guardar.

**EchoLink:** `rpt playback` también depende del modo de telemetría del enlace EchoLink. En app_rpt 3.10.8, el modo predeterminado bajo demanda puede dejar el enlace sin transmitir el archivo aunque sí lo reciban los enlaces AllStarLink. El postinstall compartido por la instalación local y el paquete Debian configura `echolinkdefault = 1` en la plantilla `[node-main](!)`, si existe, y en las secciones de `rpt.conf` indicadas por `astnode` en las secciones `elN` de `echolink.conf`. Los nodos que heredan la plantilla usan ese valor salvo que tengan uno propio; los nodos que coinciden con `astnode` se configuran explícitamente. Conserva el original en `rpt.conf.bak-echolink-telemetry` y recarga `app_rpt.so` si Asterisk está disponible. **Desconecta y reconecta los enlaces EchoLink** para que adopten el valor; el instalador no los desconecta automáticamente. Si Asterisk no estaba activo, recarga primero `app_rpt.so`. Este ajuste habilita también otra telemetría hacia EchoLink; no equivale a activar solo los archivos de la biblioteca. No hace falta activar toda la telemetría local ni reiniciar Asterisk. La prueba en 616262 confirmó recepción audible en EchoLink y un aumento de `Tx Audio` de 0 a 104 con un archivo de cinco segundos.

La comprobación y emisión usan el principal definido en el `rpt.conf` local y las credenciales AMI configuradas para ese Asterisk; no dependen de las tablas de otros servidores Allmon. No requiere crear Allmon para emitir en el Asterisk local de ese servidor. Si el principal no se puede resolver o AMI está incompleto, no emite y registra el error. En una instalación en otro servidor, «principal» es el definido por su propio `rpt.conf`, no el principal de otro equipo.

La comprobación previa mide la mezcla audible, no bloquea a las estaciones. `rpt playback` no cancela automáticamente el audio si comienza una conversación después de iniciada la emisión.

La programación corre en el servicio Go, no en el navegador, y continúa tras cerrar sesión o reiniciar Amerimon. El estado de reproducción evita solapamientos tras un reinicio del backend. Por defecto, los WAV se guardan bajo `astdatadir/sounds/amerimon`, según `/etc/asterisk/asterisk.conf`; los instaladores local y Debian preparan esa carpeta como `root:asterisk` con modo `2750`. Al actualizar, los instaladores copian allí los audios que estuvieran en la ruta anterior sin borrar los originales. Se puede cambiar la raíz en `audio.storage_dir` de `amerimon.yaml`; debe estar dentro de una ruta de sonidos que Asterisk busque, ser legible por Asterisk y escribible por Amerimon. `audio.poll_interval` controla el sondeo (por defecto `5s`; se admite de 1 a 60 segundos). El AMI del nodo debe permitir ejecutar el comando `rpt playback`.

La modalidad de bienvenida `on_connection` ya no está disponible porque la reproducción por enlace individual no está soportada de forma segura por `app_rpt`. La migración 0013 convierte las entradas existentes de esa modalidad a `broadcast_once`, sin eliminar audios ni archivos. Los audios PCM de 8 bits previamente guardados se convierten al formato de 16 bits cuando se reproducen por primera vez.

Los botones muestran iconos con descripción al pasar el cursor y etiquetas accesibles. Los auriculares (**Escuchar localmente**) descargan el WAV mediante la API autenticada para escucharlo en el navegador con su medidor, incluso si el audio está desactivado o programado. No emiten en Asterisk ni cambian la programación. El icono de emisión (**Reproducir en Asterisk**) conserva la emisión manual para audios activados de emisión única. Cambiar de escucha o salir de la pantalla libera el reproductor y su URL temporal.

Durante una solicitud, las demás acciones y campos de biblioteca y grabaciones quedan bloqueados hasta terminar, también si falla. El botón que ejecuta la acción muestra el indicador y texto de proceso; la subida incluye allí el porcentaje, sin duplicarlo afuera. Los errores y avisos de finalización permanecen visibles por separado. La grabación en curso y su sondeo automático no bloquean los controles de pausa y parada.

El panel, medidores, descripciones de iconos, errores y confirmaciones siguen el idioma de la cabecera, también al cambiar entre español e inglés sin salir de la pantalla. Los errores de audio incluyen un código `AUDIO_ERROR_*` y un mensaje de diagnóstico; conserva ambos al solicitar soporte.

#### Grabaciones del nodo principal

En **Administración → Gestión de audios**, la tarjeta **Grabaciones del nodo principal** captura la mezcla del nodo y sus enlaces. No usa el micrófono del navegador ni transmite audio a las estaciones. Requiere el contexto dedicado `amerimon-record-node`, con `Rpt(nodo,Pm)` en modo monitor de solo escucha, y el transporte `listen-live-eagi`, script EAGI y listener local. Espiar únicamente el `rxchannel` de radio puede entregar silencio aunque hablen estaciones enlazadas; la grabación no usa esa fuente. El instalador añade el contexto de grabación sin modificar el contexto de radio de Listen Live ni bloques personalizados existentes. Tras actualizar backend y dialplan, detén e inicia de nuevo una captura anterior para cambiar su fuente.

- Introduce un nombre y pulsa **Iniciar grabación**. Solo se permite una captura activa. El inicio se confirma después de recibir PCM de EAGI, no solo al aceptar el Originate AMI.
- El silencio se omite automáticamente a partir de la energía del PCM (umbral RMS aproximado de −40 dBFS), con 200 ms previos y 500 ms después de cada detección. Es detección de nivel, no reconocimiento de voz: ruido por encima del umbral también se conserva. El estado muestra **Pausa automática: esperando audio** y continúa al regresar audio; la duración muestra solo lo conservado, incluidos esos márgenes.
- **Pausar** mantiene una pausa manual aunque llegue audio. **Continuar** vuelve a habilitar la detección automática. **Detener y guardar** finaliza el WAV y cierra únicamente el canal de captura; no desconecta estaciones.
- La captura continúa al cerrar la página o la sesión web. Los WAV PCM mono de 16 bits a 8000 Hz se guardan automáticamente en el subdirectorio privado `recordings` junto a la base SQLite del servicio (directorio `0700`, archivos `0600`). El máximo por grabación es 25 MiB, aproximadamente 27 minutos de audio conservado; al alcanzarlo, se finaliza automáticamente. Las grabaciones no se borran por antigüedad.
- Si se interrumpe EAGI o se reinicia Amerimon, la captura no se reanuda sola. Se marca como interrumpida y se conserva el audio recibido; al arrancar se reconstruye la cabecera WAV a partir del PCM recuperado.
- **Escuchar** descarga el WAV por la API autenticada para reproducirlo solo en el navegador. **Añadir a biblioteca** crea una copia independiente de emisión única, sin emitir ni programar automáticamente. Puedes cambiar posteriormente su modalidad e intervalo en la tarjeta de la biblioteca. **Eliminar** pide confirmación y borra solo la grabación original; no elimina una copia ya añadida a la biblioteca.

La migración aditiva `0014_audio_recordings.sql` crea `audio_recordings` sin cambiar audios ni configuraciones existentes. Todas las rutas requieren administrador: `GET/POST /mon/audio/recordings`, `PATCH /mon/audio/recordings/:id` con `action: pause|resume|stop`, `GET /mon/audio/recordings/:id/file`, `DELETE /mon/audio/recordings/:id` y `POST /mon/audio/recordings/:id/library`.

#### Seleccionar, escuchar y guardar un fragmento

1. En la tarjeta de la biblioteca, pulsa **Configurar fragmento de emisión**. Está disponible también para audios por intervalos o desactivados. El icono de emisión manual abre el mismo cuadro.
2. Selecciona **Elegir fragmento**. Arrastra el control izquierdo para **Inicio** y el derecho para **Final**, o escribe **Inicio (s)** y **Final (s)** con precisión de 0,001 segundos. Los controles también admiten teclado y no se cruzan. La barra resaltada muestra el tramo seleccionado. Por ejemplo, inicio 5 y final 20 selecciona 15 segundos.
3. Para comprobarlo sin transmitir, pulsa los auriculares **Escuchar selección** dentro del cuadro y después reproducir en el reproductor que aparece. Incluye medidor de nivel y reproduce solo el tramo seleccionado; con **Audio completo** reproduce todo.
4. Pulsa el disquete **Guardar cambios** para recordar los límites sin emitir. El icono **Reproducir en Asterisk** guarda y después solicita la emisión; solo está disponible si el audio está activado en modo manual. **Cancelar** cierra sin guardar las ediciones pendientes.

La escucha del cuadro es exclusivamente local: no guarda límites ni transmite a Asterisk. Al cambiar los límites o la opción completo/fragmento se detiene y oculta la escucha anterior; pulsa de nuevo **Escuchar selección** para cargar la nueva selección. Al cerrar el cuadro se detiene y libera el audio.

Cuando un audio tiene límites guardados, su tarjeta muestra **Fragmento seleccionado** resaltado, con inicio, final y duración. Los límites se mantienen al recargar o reiniciar y se aplican tanto a las emisiones manuales como a cada emisión por intervalos, sin cambiar cuándo toca la próxima emisión.

Para quitar la selección, pulsa **Restablecer audio completo** y después **Guardar cambios**. La tarjeta vuelve a indicar **Audio completo**. Restablecer y cancelar no cambia los límites guardados.

El archivo de la biblioteca nunca se recorta: los auriculares de la tarjeta y la descarga siguen ofreciendo el audio completo. Solo la escucha dentro del cuadro y las emisiones usan el fragmento. La migración aditiva `0015_audio_segments.sql` guarda `start_ms` y `end_ms` por pista; los audios existentes siguen completos hasta guardar una selección. El servidor genera un WAV temporal para cada emisión del fragmento y reserva la reproducción por su duración más dos segundos; elimina el temporal un minuto después del tiempo previsto de reproducción mientras el servicio sigue activo.

#### Descarga de audios

El icono **Descargar WAV** está disponible en la biblioteca, incluso para audios desactivados o programados, y en grabaciones finalizadas con audio. Descarga mediante la API autenticada sin emitir ni modificar activaciones o intervalos. Se entrega el WAV almacenado compatible con Asterisk, no el archivo comprimido original; la conversión no conserva ese original. Durante la descarga el botón muestra **Descargando...** y bloquea otras acciones hasta completar la solicitud.

#### Medidores de señal

- **Nivel de entrada de grabación:** mide la energía RMS del PCM recibido del nodo, incluso durante la pausa manual. Se actualiza cada 500 ms mientras existe una captura activa. No representa necesariamente audio guardado: los silencios se omiten y la pausa manual descarta audio.
- **Nivel de reproducción:** mide el WAV escuchado en el navegador mediante Web Audio, con muestreo cada 50 ms, tanto en grabaciones como en biblioteca. Se reinicia al pausar o terminar. No mide la salida de Asterisk ni el volumen físico de los altavoces.
- La escala va de −60 a 0 dBFS, con zonas verde, amarilla y roja; 0 dBFS es el máximo digital. El mínimo visual es −60 dBFS. La entrada vuelve al mínimo si no llegan muestras recientes. No se requiere permiso de micrófono. Si el navegador no permite iniciar el medidor, se muestra un error explícito.

### Preferencias de advertencias y confirmaciones

Las advertencias y confirmaciones incluyen **No volver a mostrar este tipo de aviso**. La preferencia se guarda por tipo de aviso, no de forma global: ocultar la confirmación de conexión no oculta la de desconexión, ni la de desconectar todos antes de conectar. Las acciones de mantenimiento, el borrado de favoritos y enlaces externos y el guardado, importación y vaciado de listas del nodo principal tienen preferencias independientes; las listas permitida y bloqueada también se distinguen.

En una confirmación, marcar la casilla solo guarda la preferencia al confirmar la acción; cancelar no la guarda. Las siguientes solicitudes de ese tipo, iniciadas por el usuario, se ejecutan sin pedir confirmación, incluidas las acciones destructivas. En mantenimiento, tras la primera confirmación escrita válida, se conserva el contrato de confirmación del backend aunque ya no se muestre el diálogo. Ocultar avisos no ejecuta acciones por sí solo ni elimina las validaciones del servidor.

Las preferencias se almacenan en Local Storage bajo `amerimon.notice-preferences.v1`, para el mismo origen y perfil del navegador. Se conservan al recargar, cerrar sesión y volver a entrar; no se sincronizan con otros navegadores o dispositivos. **Los mensajes de error y los diagnósticos de operaciones fallidas siempre se muestran y no incluyen casilla para ocultarlos.** Si el navegador impide leer o guardar las preferencias, se muestra un error; una escritura fallida no oculta el aviso en memoria.

Temporalmente, **Admin → Personalizar cabecera → Restablecer advertencias y confirmaciones** permite volver a mostrar todos los tipos ocultos. No guarda los campos de cabecera ni modifica credenciales, sesión, nodos o configuración del servidor. El restablecimiento solo anuncia éxito si se elimina correctamente la preferencia del navegador.

### Terminal SSH

La terminal está disponible para administradores y se conecta desde el backend de Amerimon al host SSH indicado, a través del WebSocket de la aplicación. Para el mismo servidor usa `127.0.0.1` si SSH está activo; para otro servidor, indica su IP privada o nombre DNS y el puerto en el que escucha `sshd`. El destino solo necesita ser accesible desde la red del backend: **no expongas el puerto SSH a Internet**.

1. Indica host, puerto y usuario remoto, y selecciona **Contraseña** o **Clave pública**. En este último modo selecciona el archivo de clave privada de tu equipo; no introduzcas una ruta del servidor. La clave pública correspondiente debe estar autorizada para el usuario remoto. Si la clave está cifrada, introduce su frase; el botón del ojo permite mostrarla u ocultarla.
2. Para recordar las credenciales, selecciona primero la clave privada (espera a que aparezca **Clave privada lista**), introduce la frase si corresponde y pulsa **Guardar configuración y credenciales**. El mensaje de confirmación indica si se guardaron. La contraseña SSH también se puede mostrar u ocultar con el ojo.
3. La configuración de destino se guarda en el navegador; contraseñas, claves privadas y frases **no**. Al guardar credenciales, Amerimon las cifra con AES-GCM y las almacena en la tabla `terminal_credentials`, separadas por host, puerto, usuario y método. La clave maestra está en `/var/lib/amerimon/terminal-credentials.key`, separada de SQLite y con permisos `0600`. Al conectar, el backend recupera las credenciales del destino; si no existen, solicita la contraseña o la selección de la clave. Para usar esta función, accede con la sesión de administrador y protege el acceso web con HTTPS.
4. Una clave de host SSH desconocida muestra su huella y requiere confirmación explícita. Verifica la huella con el administrador del servidor antes de aceptarla; si una clave conocida cambia, la conexión se rechaza.
5. **Desconectar** cierra la sesión y limpia la pantalla y el historial de la terminal. `exit` en el shell remoto también cierra la sesión normalmente.

El comando `sudo amerimon-reset-password` elimina tanto la cuenta de administrador como las credenciales SSH guardadas; después hay que guardarlas de nuevo. No elimines ni reemplaces la clave maestra mientras existan credenciales cifradas que quieras conservar: sin la clave original no se pueden descifrar.

### Usuarios IAX y extensiones PJSIP

Accede mediante **Admin → Más opciones → Usuarios IAX / Extensiones PJSIP**, en las rutas `/admin/iax-users` y `/admin/pjsip-users`. Son páginas administrativas, no ventanas modales, y consultan su listado al abrirse. Los títulos, etiquetas, botones, estados vacíos, mensajes de carga, errores genéricos y confirmaciones de eliminación siguen el idioma español o inglés seleccionado en la cabecera, incluso al cambiarlo con el formulario abierto. Los valores de configuración no se traducen. Los diagnósticos enviados por el backend se conservan literalmente para no perder información técnica.

1. Pulsa **Nuevo usuario IAX** o **Nueva extensión PJSIP**, o selecciona una entrada del listado para editarla.
2. En IAX se editan **Nombre (sección)**, **Contexto**, **Secreto (contraseña)** y **Autenticación**. En PJSIP se editan **Número de extensión (ID)**, **Contexto**, **Contraseña**, **Identificador de llamada** y **Máximo de contactos**.
3. El nombre o ID queda bloqueado durante la edición; no se admite renombrar desde estos formularios. El contexto y el secreto o contraseña son obligatorios; el máximo de contactos debe ser un entero positivo. Los identificadores admiten de 1 a 80 letras ASCII, números, punto, guion y guion bajo; IAX reserva `general`. El backend rechaza saltos de línea, caracteres nulos y delimitadores de comentario `;` y `#` en los valores enviados.
4. **Guardar** permanece desactivado hasta que cambien los datos y el formulario sea válido; si se restauran los valores originales, vuelve a desactivarse. Al guardar aplica los cambios y vuelve a consultar la lista. Si el nombre IAX no coincide con el formato reconocido de indicativo de radioaficionado, se solicita confirmación porque la sección podría pertenecer a la configuración del sistema. Los indicativos reconocidos y las extensiones PJSIP se guardan directamente, sin esta advertencia. **Cancelar** cierra la edición sin guardar. **Eliminar** aparece al editar y solicita confirmación con el identificador de la entrada; admite nombres como `hc1lnm-m`, `hc1lnm-p` y otros nombres de usuario. La sección heredada `iaxrpt` permanece visible, pero no se puede eliminar.
5. La confirmación de guardado para secciones que no parecen indicativos solo se aplica a IAX. Se puede marcar **No volver a mostrar este tipo de aviso**; la preferencia se guarda en este navegador y, desde entonces, los guardados de ese tipo se ejecutan sin volver a preguntar. También puede restablecerse desde **Admin → Personalizar cabecera → Restablecer advertencias y confirmaciones**.
6. Si falla una operación, se muestra el error y se conservan los datos en edición para corregirlos o reintentar. **Recargar lista** reintenta la consulta sin guardar el formulario. En PJSIP, **Cancelar** descarta la edición y actualiza la lista; un guardado correcto también vuelve a la lista actualizada, tanto al crear como al editar. Guardar y eliminar quedan bloqueados mientras otra modificación está en curso.

#### Archivos y recargas

| Módulo | Archivo | Recarga por AMI |
| --- | --- | --- |
| IAX | `iax.conf` en el directorio de `ami.rpt_config_path`; por defecto `/etc/asterisk/iax.conf`. Con `[iaxclient](!)`, los usuarios nuevos se guardan en el archivo de su propio `#tryinclude` | `iax2 reload` al crear, editar o eliminar usuarios |
| PJSIP | `ami.pjsip_config_path`; por defecto `/etc/asterisk/pjsip.conf`. Si está vacío, se busca junto a `ami.rpt_config_path` | `module reload res_pjsip.so` al crear, editar o eliminar extensiones |

El usuario del servicio necesita leer y escribir los archivos y crear archivos en su directorio. Antes de guardar se crea o reemplaza un respaldo `<archivo>.amerimon.bak` con permisos `0600`; el reemplazo del archivo de configuración es atómico, resuelve los enlaces simbólicos y conserva propietario, grupo y permisos. Si falla el respaldo, no se modifica la configuración. Incluye estos archivos y respaldos, que contienen contraseñas, en una política de acceso restringido y copia de seguridad; no dependen de la base SQLite.

#### Usuarios IAX con plantilla e inclusión

Amerimon comprueba si existe la plantilla `[iaxclient](!)` en `iax.conf`. El signo `!` indica que es una plantilla, no un usuario. Si existe, busca un único `#tryinclude` **dentro de esa misma stanza**, hasta la siguiente cabecera de sección. No utiliza los comentarios ni un include de otra stanza para decidir el destino. El nombre del archivo no es fijo:

```ini
[iaxclient](!)
type = friend
context = iax-client
auth = md5
host = dynamic
#tryinclude custom/iax/iaxclient-users.conf
```

En este ejemplo los usuarios nuevos se escriben en `/etc/asterisk/custom/iax/iaxclient-users.conf`. Las rutas relativas parten del directorio de `iax.conf`; las absolutas se respetan. Si el archivo o sus directorios no existen, se crean automáticamente al guardar el primer usuario, sin tratar su ausencia como un error. El archivo nuevo usa permisos `0640`. Si ya existe, se conserva su contenido y se crea el respaldo antes de modificarlo.

Cada usuario nuevo hereda de `iaxclient` y contiene solo sus datos particulares:

```ini
[hc1lnm-m](iaxclient)
    user = hc1lnm-m
    secret = CONTRASENA_DEL_USUARIO
    callerid = "HC1LNM-M"
```

Los nombres deben ser únicos entre `iax.conf` y el archivo incluido. Se pueden usar sufijos `-m` o `-p` para distinguir clientes. Los usuarios existentes no se trasladan: los del archivo principal siguen allí, y los derivados de `iaxclient` en el incluido aparecen en el mismo listado para editar su secreto o eliminarlos, conservando las demás opciones al editar.

Sin `[iaxclient](!)`, los usuarios nuevos se crean en `iax.conf` con el formato habitual. Si existe la plantilla pero falta un `#tryinclude` válido en ella, hay varios destinos o se usan comodines, se informa del problema sin guardar en otro archivo. Después de cada creación, modificación o eliminación se envía `iax2 reload` por AMI, también si se acaba de crear el archivo incluido.

Las recargas usan las credenciales AMI configuradas en Amerimon y requieren autorización para ejecutar comandos CLI; no se ejecuta un binario `asterisk` mediante privilegios del navegador ni se reinicia Asterisk. **Si el archivo se guarda pero falla la recarga, no se revierte automáticamente:** la interfaz informa del guardado parcial. Comprueba AMI y los registros de Asterisk, revisa el contenido y recarga antes de repetir la operación. Una respuesta de recarga no garantiza que un teléfono se registre o tenga audio; verifica el resultado en Asterisk y en el cliente.

Estas páginas siguen el diseño de las demás opciones administrativas: contenedor centrado, formularios con borde, botones compactos, icono de papelera y distribución adaptable a móvil. Las contraseñas utilizan el mismo campo con botón de ojo para mostrarlas u ocultarlas, sin guardar ni modificar su valor. Se muestran ocultas inicialmente, pero se entregan al navegador de un administrador autorizado para editar la configuración; las respuestas no se almacenan en caché. Utiliza HTTPS y un dispositivo de confianza.

El icono de copia permite copiar el nombre o ID desde el listado sin abrir la edición, y el valor actual de los campos no secretos desde el formulario, sin guardarlo. Muestra confirmación o error de portapapeles. Las contraseñas y secretos no incluyen icono de copia.

#### Conservación de configuraciones personalizadas y límites

IAX no lista `[general]`, `radio`, `allstar-sys`, `allstar-public` ni plantillas marcadas con `(!)`. Las secciones reservadas se protegen también en el backend. PJSIP lista endpoints y relaciona sus referencias `auth` y `aors`, aunque los objetos tengan nombres distintos; los objetos globales y transportes no se tratan como extensiones. Al editar se conservan las opciones no expuestas, como transporte, usuario de autenticación y parámetros personalizados; las opciones expuestas se actualizan con los valores del formulario, salvo los usuarios IAX derivados de `iaxclient`, cuya edición modifica solo el secreto.

Solo se editan y eliminan secciones inequívocas del archivo principal, con la excepción IAX del archivo `#tryinclude` descrito arriba. Para PJSIP se requiere un único objeto auth `userpass` y un único AOR, ambos locales al archivo y exclusivos del endpoint. Se rechazan sin escritura los objetos compartidos, duplicados, heredados de plantillas, las referencias múltiples o ausentes y las secciones que contienen directivas `#include`. No se recorren otros archivos incluidos: una instalación distribuida entre varios archivos necesita revisión manual, también antes de crear un identificador que pudiera existir en otro include.

Las nuevas extensiones PJSIP crean objetos endpoint, auth y AOR con el mismo ID; los nuevos usuarios IAX usan los valores del formulario sin plantilla o heredan de `iaxclient` en el modo incluido. No se crean contextos del dialplan, transportes ni reglas de red: el contexto seleccionado y la infraestructura de telefonía deben existir y ser compatibles con la instalación.

### Restablecer la cuenta de administrador

Si se olvidó la contraseña, ejecuta como usuario con sudo:

```bash
sudo amerimon-reset-password
```

El comando elimina el registro de administrador y las credenciales SSH guardadas; no borra la base de datos ni los nodos, favoritos, historial de conexiones o registros Asterisk. Después, abre la web, pulsa **Admin** y vuelve a definir el usuario y la contraseña.

El reset de contraseña no rota `/etc/amerimon/jwt.secret` ni revoca tokens JWT ya emitidos. Si además se necesita invalidar todos los tokens, respalda y regenera el secreto JWT y reinicia el servicio:

```bash
sudo cp -a /etc/amerimon/jwt.secret /etc/amerimon/jwt.secret.bak
sudo sh -c 'umask 077; head -c 48 /dev/urandom | base64 -w0 > /etc/amerimon/jwt.secret'
sudo systemctl restart amerimon
```

El diálogo de acceso permite marcar **Recordar usuario, contraseña y sesión en este dispositivo**. Sin marcarlo, el JWT permanece en memoria y se pierde al recargar; al marcarlo, se almacena en Local Storage hasta cerrar sesión o recibir un rechazo del servidor. La casilla, el último usuario y su contraseña se conservan por separado en Local Storage incluso después de salir y se restauran al abrir el formulario. Un acceso correcto con la casilla desmarcada elimina la preferencia y las credenciales recordadas. Un cambio correcto de contraseña actualiza la recordada. Un acceso fallido no reemplaza las credenciales guardadas. **La contraseña se guarda sin cifrar, accesible a los scripts de la página y a quien acceda al perfil del navegador. Usa esta opción solo en dispositivos de confianza.**

La interfaz utiliza únicamente el tema oscuro, incluidos los formularios y los diagramas del manual. Los títulos, etiquetas y textos deben mantener un contraste mínimo de 4,5:1 (3:1 para texto grande); las reglas oscuras globales conservan su prioridad frente a los estilos encapsulados de Angular. La impresión conserva su formato de papel.

Si hay varios servidores, **Mostrar servidor** permite elegir un nodo local o **Todos**. El selector compacto aparece junto a **Buscar conexiones** en pantallas amplias; en móvil se apilan sin desbordar.

El pie compacto muestra la versión de Amerimon y los créditos **Powered by Angular Framework, SQLite3 y Go Language**, con logos locales adaptados al tema oscuro. No incluye enlaces legales ni contador de visitas y se oculta al imprimir.

### Enlaces externos

El botón general **Enlaces externos** está disponible para todos los visitantes y muestra únicamente registros activos. Desde **Administrar enlaces**, el administrador puede crear, editar y eliminar registros de `enlaces_externos`, con confirmación de borrado. Se conservan los campos `dashid`, `descripcion`, `url`, `target` e `is_active`; el esquema se adapta a SQLite mediante una migración nueva sin modificar los datos existentes. La descripción y URL admiten hasta 255 caracteres, se aceptan solo URL HTTP/HTTPS y los destinos `_blank` (nueva pestaña) o `_self` (misma pestaña).

El reporte de escuchas de Listen Live ya no muestra las columnas **Usuario** e **Indicativo**. Se conservan ID, IP de origen, inicio y las acciones autorizadas.

| Ruta | Acceso | Función |
| --- | --- | --- |
| `/mon/health` | Público | Salud de HTTP/DB |
| `/mon/auth/login`, `/mon/auth/status`, `/mon/auth/setup` | Público | Inicio y configuración inicial de administrador |
| `/mon/allstarlink/nodes`, `/mon/allstarlink/activity`, `/mon/allstarlink/activity/live` | Público, token opcional | Nodos y actividad (REST y WebSocket en tiempo real) |
| `/mon/listen-live/*` | Público, token opcional según operación | Sesiones, estado y WebSocket de audio |
| `/mon/allmon/config` | Administrador | Leer/guardar nodos Allmon |
| `/mon/iax-users` | Administrador | GET: mapa de usuarios por nombre; POST: crear o actualizar un usuario IAX |
| `/mon/iax-users/:name` | Administrador | DELETE: eliminar la sección IAX solo si su nombre tiene formato de indicativo de radioaficionado |
| `/mon/pjsip-users` | Administrador | GET: lista de endpoints con auth/AOR; POST: crear o actualizar una extensión |
| `/mon/pjsip-users/:id` | Administrador | DELETE: eliminar endpoint y sus objetos auth/AOR exclusivos |
| `/mon/asterisk/commands` | Administrador | Listar y ejecutar acciones Asterisk |
| `/mon/asterisk/logs` | Administrador | Últimas 500 operaciones por defecto, máximo 2000 |
| `/mon/audio/tracks/:id/file` | Administrador | GET: WAV autenticado para escucha local, sin emitir ni cambiar la programación |
| `/mon/audio/tracks` | Administrador | GET: biblioteca; POST: subir audio y convertir a WAV |
| `/mon/audio/tracks/:id` | Administrador | PATCH: editar modalidad/activación; DELETE: eliminar audio |
| `/mon/audio/tracks/:id/play` | Administrador | Iniciar emisión única |
| `/mon/allstarlink/connections/history` | Administrador | Historial de conexiones |
| `/mon/system-info` | Administrador | Sistema, CPU, memoria, discos, red, temperatura, voltaje y Asterisk |
| `/mon/site-settings` | GET público; PUT administrador | Título/subtítulo de cabecera |
| `/mon/favoritos` | Administrador | Favoritos |
| `/mon/enlaces-externos` | Público | Solo enlaces activos |
| `/mon/admin/enlaces-externos`, `/mon/admin/enlaces-externos/:id` | Administrador | CRUD de enlaces externos |

## 7. Base de datos, migración y respaldo

La DB del servicio instalado es `/var/lib/amerimon/amerimon.db`. Cambiar `AMERIMON_DB` selecciona otro archivo.

Las migraciones se aplican al abrir la base. Si ya hay migraciones aplicadas, antes de aplicar nuevas se crea un backup con sufijo de fecha y se conservan las tres copias más recientes.

Para una copia manual consistente:

```bash
sudo systemctl stop amerimon
sudo cp -a /var/lib/amerimon/amerimon.db \
  "/var/lib/amerimon/amerimon.db.manual-$(date +%Y%m%d-%H%M%S)"
sudo systemctl start amerimon
```

Para restaurar, detén el servicio, conserva una copia de la DB actual, reemplaza el archivo por el backup y vuelve a iniciarlo. Mantén propietario/permisos root y revisa el estado con `journalctl`.

## 8. Tecnologías y datos

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
| `terminal_credentials` | Credenciales SSH cifradas por destino |
| `asterisk_operation_logs` | Operaciones Asterisk, resultado y diagnóstico |
| `audio_tracks` | Biblioteca, formato, activación y duración calculada de los audios |
| `audio_broadcast_schedules` | Intervalo y próximas/últimas reproducciones programadas |
| `audio_playback_state` | Reserva persistente para evitar solapamientos de audio |

## 9. Registro de operaciones Asterisk

Las operaciones del panel AllStarLink se guardan en `asterisk_operation_logs` y aparecen en **Administración → Registros Asterisk**. Se almacenan fecha UTC, acción, nodo local/remoto, modo, comando enviado, resultado, detalle y duración. Los detalles se limitan a 2000 caracteres.

Los resultados distinguen, entre otros, solicitud inválida, error de configuración, error AMI, comando aceptado pero enlace no confirmado y éxito. Los logs de proceso también usan el prefijo `asterisk:` y pueden consultarse con:

```bash
sudo journalctl -u amerimon -f | grep 'asterisk:'
```

No se guardan `AMI_SECRET`, contraseñas ni tokens. El endpoint y la página de registros están restringidos al administrador. La lista ofrece hasta 2000 eventos; no hay purga automática en la aplicación, así que la base debe incluirse en la política de respaldo/retención.

Este historial cubre las acciones del panel de comandos/conexiones y los intentos de inicio/parada de Listen Live (`listen_start`, `listen_stop`), incluidos los errores AMI. Un inicio correcto indica que AMI aceptó `Originate`, no que ya llegue audio. No reemplaza los logs de Asterisk.

La gestión de audios registra emisiones únicas y programadas y errores AMI. El tiempo de espera entre reproducciones se calcula de la duración WAV verificada y se mantiene en `audio_playback_state`.

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

Las solicitudes y sus resultados quedan en **Registros Asterisk** y en el journal de Amerimon. El servicio instalado se ejecuta como root; una instancia sin esos permisos devolverá el error del sistema, sin intentar elevar privilegios. No se deben probar las acciones Linux sobre un equipo compartido o en producción sin autorización.

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

La instalación habilita `proxy`, `proxy_http`, `proxy_wstunnel`, `headers` y `alias`, activa `conf-available/amerimon.conf`, ejecuta `apache2ctl configtest` y solo recarga/inicia Apache si la prueba pasa. El proxy cubre HTTP/HTTPS según el VirtualHost que reciba la petición, incluidas las cabeceras de prefijo y los WebSockets de actividad AllStarLink, Listen Live y terminal.

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

## 13. Diagnóstico rápido

| Síntoma | Comprobaciones |
| --- | --- |
| Servicio no inicia | `systemctl status amerimon`; `journalctl -u amerimon -n 100`; revisar `8082` y `8086` |
| Nodo local no aparece | Confirmar `rpt_config_path`, las secciones numéricas y la entrada loopback del nodo principal en `rpt.conf`; Allmon solo aporta servidores remotos |
| AMI rechaza operación | Revisar `AMI_USER`/`AMI_SECRET` sin imprimirlos; validar host/puerto y permisos del usuario existente; consultar `journalctl -u amerimon` |
| IAX/PJSIP no carga o no guarda | Revisar las rutas de configuración, permisos de archivo/directorio y sesión de administrador; usar **Recargar lista** y leer el diagnóstico |
| IAX/PJSIP devuelve conflicto al editar o eliminar | Revisar duplicados, plantillas, includes, referencias auth/AOR ausentes, múltiples o compartidas; resolver manualmente sin eliminar opciones del cliente |
| IAX/PJSIP indica archivo guardado pero recarga fallida | El archivo ya cambió; comprobar AMI y registros de Asterisk, revisar el contenido y recargar antes de reintentar; no hay reversión automática |
| AMI acepta, pero no confirma conexión | Abrir Registros Asterisk y revisar detalle de `XStat`, estado remoto y timeout |
| Aviso de mismo modo y permanencia | El enlace ya tiene la modalidad solicitada; no se enviaron comandos. Cambiar modo o permanencia solo si realmente se desea reconectar |
| No se inició la nueva conexión al cambiar modalidad | Revisar en Registros Asterisk la desconexión anterior, la confirmación de ausencia en `XStat` y posibles rutas alternativas; no se restaura automáticamente el enlace retirado |
| `/amerimon` devuelve 404 | Confirmar conf Apache/Nginx habilitada, ruta, `configtest` y proxy del VirtualHost correcto |
| `apt` ofrece versión anterior | Ejecutar `apt update`, revisar `apt-cache policy amerimon` y confirmar el índice público de Pages |
| Temperatura/voltaje no disponible | Verificar sensores del kernel, `vcgencmd` en ARM o entradas `hwmon` en AMD |

No se debe editar Asterisk para resolver un error del proxy web; tampoco exponer el puerto AMI `5038` fuera del host sin un diseño de seguridad específico.
