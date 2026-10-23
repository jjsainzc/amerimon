
# Opciones admin
## 1. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch

## 2. En la parte de admin, nodo principal, agregar mantenimiento para esa stanza en whitelist/blacklist de la misma formato que la que existe de permitidos/denegados, el formato en la base es:

```
/whitelist/HC1LNM                             : JJSC
```
`database put <whilelist|blacklist> <callsing> <motivo>`


## 3. En las configuraciones de nodo principal, antes de la lista de accesos, para el file /etc/asterisk/manager.conf,  agregar cambio de passwd, hay que mostrarlo,  AMI (secret) y bind address, opcional las variables deny y permit para agregar redes con su ayuda de calculos de red, usar "manager reload" para no reiniciar asterisk, y actualizar esto en las config del yaml /etc/amerimon y en la parte de los accesos AMI para el nodo principal.



> **NOTA:** Las actualizaciones no deben quitar las configuraciones personalizadas del cliente, si hay un cambio en las mismas, estas deben agregarse sin cambiar o borrar las existentes 
