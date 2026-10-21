# Depurar Angular y Go en VS Code

## 1. Preparar el entorno

- Abre la carpeta del proyecto en VS Code.
- Instala Go con una versión compatible con [go.mod](../go.mod).
- Si aún no has instalado las dependencias de Angular, ejecuta desde la raíz:

  ```bash
  npm ci
  ```

- Instala la extensión **Go** (`golang.go`). Si trabajas por SSH o WSL, instálala también en ese entorno.
- Si VS Code solicita instalar **Delve**, acepta. Es el depurador del backend Go.

Las configuraciones ya están en [launch.json](../.vscode/launch.json) y [tasks.json](../.vscode/tasks.json).

## 2. Liberar los puertos

Antes de iniciar otra ejecución, comprueba los puertos:

```bash
ss -ltn '( sport = :4200 or sport = :8082 or sport = :8086 )'
```

Detén las sesiones anteriores y sus tareas de desarrollo. Si está activo el servicio instalado de Amerimon, detenlo:

```bash
sudo systemctl stop amerimon
```

No ejecutes `make dev-backend` al mismo tiempo que el depurador: ambos intentarían iniciar el backend.

## 3. Iniciar la depuración conjunta

1. Abre **Ejecutar y depurar** con `Ctrl+Shift+D`.
2. En el selector de configuraciones, elige **Angular + Go**.
3. Pulsa **F5**.
4. Usa la aplicación en `http://localhost:4200/`.

La configuración conjunta inicia:

| Componente | Dirección o puerto | Función |
| --- | --- | --- |
| Angular | `http://localhost:4200/` | Interfaz y depuración TypeScript |
| Go | `http://127.0.0.1:8082` | API y depuración del backend |
| Audio interno | `127.0.0.1:8086` | Recepción de audio EAGI |

Angular reenvía las peticiones `/mon` al backend mediante [proxy.conf.json](../proxy.conf.json). Es normal que el navegador muestre el puerto `4200` aunque Go también esté ejecutándose.

## 4. Usar breakpoints

1. Abre el archivo TypeScript o Go que quieras inspeccionar.
2. Haz clic en el margen izquierdo de una línea ejecutable para colocar un breakpoint.
3. Realiza desde el navegador la acción que ejecuta ese código.
4. Cuando se detenga, inspecciona **Variables**, **Inspección** y **Pila de llamadas**.

Atajos habituales:

| Tecla | Acción |
| --- | --- |
| `F5` | Continuar |
| `F10` | Paso a paso por procedimientos |
| `F11` | Entrar en una función |
| `Shift+F11` | Salir de una función |
| `Shift+F5` | Detener la depuración |

Un breakpoint Go se activa cuando una petición o tarea ejecuta esa línea; abrir la página no necesariamente ejecuta todos los controladores.

## 5. Configuración y credenciales

El backend del depurador usa:

- Configuración: [packaging/amerimon.yaml](../packaging/amerimon.yaml).
- Base SQLite: `/tmp/amerimon-dev.db`, separada de la base del servicio instalado.
- Secreto JWT: `dev-secret`, exclusivamente para desarrollo.

Antes de iniciar el backend, la tarea **Preparar AMI para depurar** ejecuta [prepare-debug-env.cjs](../scripts/prepare-debug-env.cjs):

1. Usa `AMI_USER` y `AMI_SECRET` si están definidos en el entorno de VS Code.
2. Si faltan, lee `/etc/asterisk/manager.conf` y selecciona la primera cuenta con `secret` definido directamente. Si ya definiste `AMI_USER`, busca esa cuenta. Las cuentas basadas únicamente en plantillas o archivos incluidos requieren definir ambas variables en el entorno.
3. Genera `.vscode/ami.env` con permisos `0600`, excluido de Git, para que el depurador cargue las credenciales.

Si el archivo no es legible o no encuentra una cuenta, la tarea falla y muestra la causa. No ejecutes VS Code como root: configura ambas variables en su entorno si no puedes leer `manager.conf`. En SSH/WSL, deben estar disponibles en el entorno remoto que ejecuta el depurador.

No guardes credenciales reales en los archivos de configuración versionados ni uses `dev-secret` en producción.

### Dirección del servidor AMI

Para configurar nombres en la red, consulta [Usar hostname en la LAN en lugar de una IP](hostname-lan.md).

El campo `ami.host` del YAML y el campo `host` de los servidores remotos en Allmon admiten IP o hostname, sin `http://` y con el puerto configurado por separado. Por ejemplo: `127.0.0.1`, `192.168.1.50`, `::1` o `asterisk.example.net`.

El backend resuelve cada hostname antes de conectar. Para nombres `.local` usa `getent ahosts` y el soporte NSS/Avahi del sistema, incluso en binarios sin cgo; para los demás usa el resolvedor de Go. Si obtiene varias direcciones, intenta las siguientes si falla la conexión, compartiendo el timeout entre la resolución y los intentos TCP. Las IP literales se usan directamente. Los fallos de resolución incluyen el hostname en el error del backend.

Para depurar ese paso, coloca un breakpoint en `dialAMI` dentro de [asteriskami.go](../internal/config/asteriskami.go). Inspecciona `host`, `addresses` y, tras conectar, `conn.RemoteAddr()` para ver la IP utilizada.

**Importante:** las rutas de Asterisk y Allmon siguen siendo las indicadas en el YAML. Aunque la base SQLite sea de desarrollo, las acciones administrativas pueden modificar archivos y nodos reales.

## 6. Detener y restaurar el servicio

1. Pulsa **Detener** o `Shift+F5`. La configuración conjunta finaliza ambas sesiones de depuración.
2. La tarea Angular puede seguir activa. Usa la paleta de comandos (`Ctrl+Shift+P`), selecciona **Tareas: Terminar tarea** y termina `npm: start`.
3. Si detuviste el servicio instalado, restáuralo:

   ```bash
   sudo systemctl start amerimon
   sudo systemctl status amerimon --no-pager
   ```

## Diagnóstico rápido

- **Solo inicia Angular:** comprueba que seleccionaste **Angular + Go**, no **ng serve**.
- **Puerto ocupado:** detén la ejecución anterior o el servicio correspondiente; no inicies dos backends.
- **Go no permite iniciar la depuración:** comprueba que la extensión Go y Delve estén instalados en el entorno donde se ejecuta el proyecto.
- **Las funciones AMI fallan:** revisa la salida de **Preparar AMI para depurar**, los permisos de la cuenta AMI y la conectividad con Asterisk. Después de cambiar las credenciales, detén Go y vuelve a iniciar con F5.
- **Comprobar backend y proxy:** con ambos componentes ejecutándose, estas peticiones deben devolver `{"status":"ok"}`:

  ```bash
  curl -fsS http://127.0.0.1:8082/mon/health
  curl -fsS http://localhost:4200/mon/health
  ```

También puedes seleccionar **Backend Go** para depurar únicamente Go o **ng serve** para depurar únicamente Angular. Antes de iniciar **Backend Go** por separado, ejecuta **Tareas: Ejecutar tarea → Preparar AMI para depurar**. La configuración conjunta **Angular + Go** ya ejecuta esa tarea antes de cargar las credenciales en el depurador.
