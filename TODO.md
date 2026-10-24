
# Opciones admin
## 1. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch

## 2. En la parte de admin, nodo principal, agregar mantenimiento para esa stanza en whitelist/blacklist de la misma formato que la que existe de permitidos/denegados, el formato en la base es:

```
/whitelist/HC1LNM                             : JJSC
```
`database put <whilelist|blacklist> <callsing> <motivo>`


## 4. En el combo desplegable de mas opciones, personalizar cabecera y cambia contraseña se ven como botones, ponerlo igual que los demás.



> **NOTA:** Las actualizaciones no deben quitar las configuraciones personalizadas del cliente, si hay un cambio en las mismas, estas deben agregarse sin cambiar o borrar las existentes 
