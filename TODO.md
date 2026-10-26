
# 1. General

## 1.2. Cada vez que se de click a una pagina poner un cuadrito con icono girando que diga "Cargando ...".

# 2. Opciones admin

## 2.1. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch

## 2.2. En la parte de admin, nodo principal, agregar mantenimiento para esa stanza en whitelist/blacklist de la misma formato que la que existe de permitidos/denegados, el formato en la base es:

```
/whitelist/HC1LNM                             : JJSC
```
`database put <whilelist|blacklist> <callsing> <motivo>`

## 2.3. Completar traducción.

# 3. Bugs

 


> **NOTA:** Las actualizaciones no deben quitar las configuraciones personalizadas del cliente, si hay un cambio en las mismas, estas deben agregarse sin cambiar o borrar las existentes 
