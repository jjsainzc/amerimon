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

Para funciones que requieren Asterisk/AMI, define `AMI_USER` y `AMI_SECRET` en el entorno del proceso de VS Code antes de abrirlo. En SSH/WSL, deben estar disponibles en el entorno remoto que ejecuta el depurador. Esta configuración no extrae las credenciales de `manager.conf`, a diferencia de `make dev-backend`.

No guardes credenciales reales en los archivos de configuración versionados ni uses `dev-secret` en producción.

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
- **Las funciones AMI fallan:** revisa las variables de entorno, los permisos y la conectividad con Asterisk.
- **Comprobar backend y proxy:** con ambos componentes ejecutándose, estas peticiones deben devolver `{"status":"ok"}`:

  ```bash
  curl -fsS http://127.0.0.1:8082/mon/health
  curl -fsS http://localhost:4200/mon/health
  ```

También puedes seleccionar **Backend Go** para depurar únicamente Go o **ng serve** para depurar únicamente Angular.
