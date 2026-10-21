# Reorganizar header
## 1. Poner botones en menu desplegable, (dejar solo los mas relevantes afuera)  y el boton salir ponerlo con in icon al extremo derecho en la linea donde estan Allstarlink, Listen-Live y Enlaces externos

# Opciones admin
## 1. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch

## 2. En la parte de admin, nodo principal, agregar mantenimiento para esa stanza en whitelist/blacklist de la misma formato que la que existe de permitidos/denegados, el formato en la base es:

```
/whitelist/HC1LNM                             : JJSC
```
`database put <whilelist|blacklist> <callsing> <motivo>`

## 3. Agregar un check mark en lista de nodos allmon para desactivar su seguimiento (visualizacion/edicion) pero mantenerlos en la lista.

## 4. En las configuraciones de nodo principal, para el file /etc/asterisk/manager.conf,  agregar cambio de passwd, hay que mostrarlo,  AMI (secret) y bind address, opcional las variables deny y permit para agregar redes con su ayuda de calculos de red, usar "manager reload" para no reiniciar asterisk, y actualizar esto en las config del yaml /etc/amerimon.

## 5. Tratar el host de la lista de nodos allmon como ip y como hostname.
