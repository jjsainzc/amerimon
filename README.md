# Amerimon

Interfaz web para consultar AllStarLink, administrar nodos Allmon y escuchar el nodo principal de Asterisk.

El monitor consulta cada servidor del grupo de forma independiente. Si un servidor no responde, reintenta su conexión en segundo plano cada 5 segundos sin bloquear los demás, incluso con la actualización automática desactivada. El aviso superior de conexión corresponde únicamente al nodo principal; los errores de otros servidores aparecen en su propio grupo. Los errores al obtener la configuración del monitor se muestran como errores generales.

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

## Primer acceso

Abre `http(s)://<host>/amerimon/` con el proxy configurado, o `http://<IP-del-servidor>:8082/` si accedes directamente. Pulsa **Admin** y define el usuario y contraseña iniciales. El nombre de usuario debe tener entre 3 y 32 caracteres; la contraseña debe tener al menos 8. La contraseña se guarda como hash.

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

## Manual técnico

Para configuración de nodos, puertos, SQLite, proxy, respaldos, operaciones administrativas y diagnóstico, consulta el [Manual de Operación Técnica](docs/manual-operacion-tecnica.md).
