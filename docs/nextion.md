# Controlador Nextion

La pantalla se configura en **Administración → Pantalla Nextion**. Amerimon guarda
el estado de activación, el dispositivo serie, la velocidad y el nodo favorito en
SQLite. El nodo local y las credenciales AMI se reutilizan desde la configuración
existente de Asterisk/Amerimon. Es posible guardar valores incompletos mientras el
controlador esté desactivado; al activarlo se validan todos los campos necesarios.

La lista del puerto muestra los dispositivos `/dev/ttyUSB*` y `/dev/ttyACM*`.
Cuando el USB expone el identificador `10c4:ea60`, se identifica como adaptador
Silicon Labs CP210x. El botón de prueba abre el dispositivo con la velocidad
elegida para confirmar que esté disponible y lo cierra inmediatamente; no envía
comandos a la pantalla ni la ocupa de manera permanente.

La interfaz de red que proporciona la dirección IP puede seleccionarse entre
las interfaces activas no loopback; cada opción muestra su IPv4 y MAC. La IPv4
actual también aparece debajo del selector. Si se deja en automática, Amerimon
elige una dirección IPv4 disponible. El nodo local se detecta desde la
configuración RPT de Asterisk y el nodo favorito se elige de la lista de Favoritos.

## Variables de pantalla

Los nombres son componentes/properties de Nextion; `txt` significa texto y `bco`
es el color de fondo. Todos los componentes referenciados por código deben tener
`vscope=global`.

| Variable | Contenido |
| --- | --- |
| `t_nodo.txt` | Número del nodo local, con el prefijo `Nodo Local:`. |
| `t_ip.txt` | Dirección IPv4 seleccionada automáticamente. |
| `t_temp.txt` | Temperatura de CPU en °C; `--` cuando no se puede leer. |
| `t_links.txt` | Título `NODOS DIRECTOS` y cantidad adicional cuando hay más de cuatro enlaces. |
| `t0.txt` | Estado de Asterisk: listo, recibiendo, transmitiendo o mensaje temporal del sistema. |
| `b_estado.bco`, `t0.bco` | Color del estado: gris `33840` (espera), rojo `63488` (TX), verde `1024` (RX). |
| `t_n1.txt`–`t_n4.txt` | Los cuatro primeros nodos conectados directamente; `--` si la posición está vacía. |
| `b_d1`–`b_d4` | Botones transparentes sobre sus respectivos nodos; solo son visibles cuando hay un enlace. |
| `va_alive.val` | Latido que Amerimon actualiza a `4` cada segundo. |

## Botones y eventos

Configurar cada evento en **Touch Release Event** para que envíe exactamente la
cadena indicada (sin comillas ni terminador adicional). Los botones físicos de
la pantalla pueden usar iconos; el controlador procesa los nombres de evento.

| Botón / uso | Cadena | Acción |
| --- | --- | --- |
| Conectar favorito | `CONN_FAV` | Conecta el nodo favorito configurado, si no está conectado. |
| Desconectar todos | `DISC_ALL` | Ejecuta el equivalente a `*70` (`ilink 6`) en el nodo local. |
| Mantener pulsada la fila 1–4 | `HOLD_N<número>` + espacio + `<nodo>` + LF | Registra temporalmente el nodo de esa fila para permitir su desconexión. |
| Soltar la fila 1–4 | `DISC_N<número>` | Desconecta ese nodo si la fila se había mantenido pulsada y el enlace sigue activo. |
| Preparar reinicio de Asterisk | `SYS_ARM_RESTART` | Arma la acción durante 10 segundos; requiere `SYS_CONFIRM` para ejecutarla. |
| Preparar reinicio de Linux | `SYS_ARM_REBOOT` | Arma el reinicio durante 10 segundos; requiere `SYS_CONFIRM`. |
| Preparar apagado de Linux | `SYS_ARM_OFF` | Arma el apagado durante 10 segundos; requiere `SYS_CONFIRM`. |
| Confirmar acción de sistema | `SYS_CONFIRM` | Ejecuta una acción armada que no haya expirado. |
| Cancelar acción de sistema | `SYS_CANCEL` | Cancela la acción armada. |

Las cadenas `DISC_N1`–`DISC_N4` por sí solas no bastan para desconectar: el
controlador exige primero el evento de pulsación larga con el número del nodo.
La respuesta estándar `sendme` de Nextion reinicia el estado de interfaz al
reconectarse la pantalla.

## Archivos por modelo e idioma

Cada pantalla e idioma se distribuye con el mismo nombre base, que incluye el
código de idioma antes de la extensión:

- `public/assets/firmware/AMERINET_Nextion_<modelo>-<idioma>.HMI` (fuente editable)
- `public/assets/firmware/AMERINET_Nextion_<modelo>-<idioma>.tft` (firmware para SD)
- `public/assets/images/AMERINET_Nextion_<modelo>-<idioma>.png` (vista previa)

Por ejemplo, `AMERINET_Nextion_NX4832K035-es.HMI`,
`AMERINET_Nextion_NX4832K035-es.tft` y
`AMERINET_Nextion_NX4832K035-es.png` son el fuente, firmware y vista previa del
NX4832K035 en español. Se pueden agregar otros modelos e idiomas siguiendo este
formato. El catálogo administrativo agrupa archivos por nombre base, muestra la
vista previa y ofrece botones de icono independientes para descargar el TFT y,
cuando exista, el fuente HMI. El TFT se copia a una tarjeta SD para actualizar la
pantalla. Los archivos se instalan en `/usr/share/amerimon/nextion/`; el directorio
se excluye de los assets públicos del build Angular y se sirve mediante la ruta
administrativa autenticada.
