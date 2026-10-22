# General 


## 1. Reorganizar header, poner botones en menu desplegable, para admin,  (dejar solo los mas relevantes afuera, Sistema, Favoritos, Administrar enlaces)  y el boton salir ponerlo con in icon al extremo derecho en la linea donde estan Allstarlink, Listen-Live y Enlaces externos.

## 2. Agregar descripcion, ip, hostname en la cabecera de cada tabla de cada nodo remoto al lado de su numero.

## 3. Agregar una linea de weather segun el timezone local (tomado del browser, no del server), aca hay un ejemplo /usr/local/sbin/supermon/weather.sh y poner la configuracion de la visualizacion y modificacion de sus parametros en admin, como localizacion (si desea cambiarlo), las unidades de medida de temp F/C de velocidades Kmh-Mph, presion hPa, si se dan en ambas unidades respectivamente.

# Opciones admin
## 1. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch

## 2. En la parte de admin, nodo principal, agregar mantenimiento para esa stanza en whitelist/blacklist de la misma formato que la que existe de permitidos/denegados, el formato en la base es:

```
/whitelist/HC1LNM                             : JJSC
```
`database put <whilelist|blacklist> <callsing> <motivo>`

## 3. Agregar un check mark en lista de nodos allmon para desactivar su seguimiento (visualizacion/edicion) pero mantenerlos en la lista.

## 4. En las configuraciones de nodo principal, para el file /etc/asterisk/manager.conf,  agregar cambio de passwd, hay que mostrarlo,  AMI (secret) y bind address, opcional las variables deny y permit para agregar redes con su ayuda de calculos de red, usar "manager reload" para no reiniciar asterisk, y actualizar esto en las config del yaml /etc/amerimon.

