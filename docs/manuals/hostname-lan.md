# Usar hostname en la LAN en lugar de una IP

## Qué nombre usar

El comando `hostname` muestra el nombre del equipo, pero no registra automáticamente ese nombre en el DNS de la red.

Para una LAN pequeña, puedes usar **mDNS con Avahi** y conectar mediante `nombre.local`. Para varias subredes o VLAN, conviene configurar un **DNS local**.

En este entorno se comprobó desde el servidor Linux de Amerimon:

| Dirección | Resultado |
| --- | --- |
| `rpi4d13` | No se resuelve |
| `rpi4d13.local` | Resuelve a `192.168.100.74` |
| `rpi4d13.local:5038` | Acepta conexión TCP |

La IP puede cambiar con DHCP; la aplicación debe guardar el hostname, no esa IP. La prueba TCP no verifica el usuario ni la contraseña de AMI.

## Opción recomendada para esta LAN: mDNS / Avahi

### 1. En el equipo al que quieres conectar

En la Raspberry Pi con Asterisk, comprueba el nombre y el servicio:

```bash
hostname
systemctl status avahi-daemon --no-pager
```

Si Avahi no está instalado, en Debian, Ubuntu o Raspberry Pi OS:

```bash
sudo apt update
sudo apt install avahi-daemon
sudo systemctl enable --now avahi-daemon
```

Si necesitas cambiar el nombre del equipo, elige uno único en la LAN:

```bash
sudo hostnamectl set-hostname rpi4d13
```

Solo si cambiaste el nombre, revisa con `sudoedit /etc/hosts` que la entrada local existente para el nombre anterior, normalmente en `127.0.1.1`, use el nuevo nombre. Conserva las entradas de `localhost`. Después reinicia Avahi:

```bash
sudo systemctl restart avahi-daemon
```

Si `hostname` ya devuelve `rpi4d13` y `rpi4d13.local` se resuelve, no hace falta cambiar nada en ese equipo.

### 2. En el servidor donde corre el backend Go

Haz las comprobaciones aquí, no solamente en el PC desde el que usas SSH:

```bash
getent ahosts rpi4d13.local
```

Si no se resuelve, comprueba el soporte mDNS. En un sistema Debian/Ubuntu que use Avahi y NSS, instala los componentes que falten:

```bash
sudo apt update
sudo apt install avahi-daemon avahi-utils libnss-mdns
sudo systemctl enable --now avahi-daemon
```

Comprueba:

```bash
grep '^hosts:' /etc/nsswitch.conf
avahi-resolve-host-name -4 rpi4d13.local
getent ahosts rpi4d13.local
```

En sistemas con `libnss-mdns`, una línea habitual es:

```text
hosts: files myhostname mdns4_minimal [NOTFOUND=return] dns
```

Es un ejemplo, no una línea para reemplazar a ciegas: conserva los módulos propios de tu distribución. Si el sistema usa `systemd-resolved`, mDNS puede gestionarse mediante ese servicio en lugar de Avahi/NSS.

Amerimon usa `getent ahosts` para los nombres terminados en `.local`, también en los binarios estáticos compilados con `CGO_ENABLED=0`. Así consulta NSS/Avahi sin depender de cgo ni enviar esos nombres al DNS público. Los demás hostnames siguen usando el resolvedor de Go y las IP se usan directamente.

El paquete Debian declara las dependencias `libc-bin`, `libnss-mdns` y `avahi-daemon`. Si instalas con `install.sh`, prepara esos componentes mediante los comandos anteriores: el script comprueba que exista `getent`, pero no instala ni configura Avahi/NSS.

Si el servicio todavía registra `lookup ...local on 1.1.1.1:53`, está ejecutando una versión anterior a esta corrección. Recompila e instala con `bash install.sh` y comprueba los logs del servicio reiniciado. Cambiar el navegador o acceder desde fuera de la LAN no cambia el resolvedor del backend.

### 3. Comprobar red y puerto AMI

mDNS normalmente funciona dentro de la misma red local y usa multicast UDP `5353`. La red Wi-Fi, el firewall o el aislamiento de clientes no deben bloquearlo. Entre VLAN o subredes no suele funcionar sin configuración adicional; en ese caso usa DNS local.

Después de resolver el nombre, si tienes Netcat instalado, prueba el puerto:

```bash
nc -vz -w 5 rpi4d13.local 5038
```

Si resuelve pero el puerto no responde, revisa la escucha de AMI en el equipo Asterisk, las reglas del firewall y sus restricciones de acceso. No abras AMI a Internet ni desactives sus controles de acceso.

### 4. Configurar Amerimon

Para el nodo remoto, abre **Admin → configuración de Allmon** y guarda:

```text
Host:   rpi4d13.local
Puerto: 5038
```

Usa el usuario y la contraseña AMI de ese servidor. No pongas `http://`, `ssh://` ni el puerto dentro del campo Host.

El monitor vuelve a consultar el servidor en segundo plano. Si cambiaste `ami.host` en el YAML en lugar de un servidor remoto de Allmon, reinicia el backend para cargar el cambio.

El código actual no agrega `.local` automáticamente: escribe el nombre completo `rpi4d13.local`.

## Alternativa: DNS local del router o servidor de red

Para no depender de mDNS:

1. Configura el router o DNS local para registrar los nombres de los equipos a partir de sus concesiones DHCP, o crea registros DNS con reservas DHCP.
2. Usa un dominio local apropiado, por ejemplo `home.arpa` para una red doméstica. No uses `.local` como zona DNS unicast: está reservado para mDNS.
3. Haz que DHCP entregue ese DNS local a los clientes, incluido el servidor de Amerimon.
4. Verifica desde el servidor de Amerimon:

   ```bash
   getent ahosts rpi4d13.home.arpa
   ```

5. Guarda `rpi4d13.home.arpa` como Host en Amerimon.

Para usar solamente `rpi4d13`, el resolvedor necesita que ese nombre exista directamente o un dominio de búsqueda como `home.arpa`. Configura ese dominio mediante DHCP o el gestor de red.

Servidores DNS públicos como `1.1.1.1` o `8.8.8.8` no conocen los nombres privados de tu LAN. No edites directamente un `/etc/resolv.conf` generado por el sistema: configura el DNS en el router o gestor de red.

## Alternativa manual: /etc/hosts

Si no puedes usar mDNS ni DNS local, agrega una entrada en `/etc/hosts` del servidor que ejecuta Amerimon:

```text
192.168.100.74 rpi4d13
```

Esto permite usar `rpi4d13` en la aplicación, pero **sigue dependiendo de una IP fija o reservada**. Debes actualizar la entrada si cambia la IP y repetirla en cada equipo que necesite resolver ese nombre.

## Diagnóstico rápido

| Síntoma | Qué revisar |
| --- | --- |
| SSH funciona en Windows, pero Amerimon no resuelve | Windows y Linux pueden usar mecanismos distintos; prueba en el servidor del backend |
| `rpi4d13` falla y `rpi4d13.local` funciona | Usa el nombre completo con `.local` |
| El nombre resuelve, pero TCP `5038` falla | Escucha AMI, firewall, ruta de red y permisos de acceso |
| TCP funciona, pero AMI rechaza el login | Usuario, contraseña y configuración AMI del servidor remoto |
| Funciona en una LAN, pero no entre VLAN | Usa DNS local o configura explícitamente la infraestructura mDNS |

Para iniciar el depurador, consulta la [guía de depuración en VS Code](depuracion-vscode.md).
