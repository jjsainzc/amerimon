
# 1. General

## 1.2. Cada vez que se de click a una pagina poner un cuadrito con icono girando que diga "Cargando ...".

## 1.3. El clima no se esta ejecutando automaticamente cuando se carga la pagina o cuando cambia de idioma, hay que hacerlo manual.

## 1.4. Habilitacion del pjsip, modulos y patch en el extensions si es que hace crash 

# 2. Opciones admin

## 2.1. Quitar personalizar cabecera y hacer un perfil de usuario admin para configuraciones generales.

## 2.2. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch

## 2.3. En la parte de admin, nodo principal, agregar mantenimiento para esa stanza en whitelist/blacklist de la misma formato que la que existe de permitidos/denegados, el formato en la base es:

```
/whitelist/HC1LNM                             : JJSC
```
`database put <whilelist|blacklist> <callsing> <motivo>`



# 3. Bugs

 


> **NOTA:** Las actualizaciones no deben quitar las configuraciones personalizadas del cliente, si hay un cambio en las mismas, estas deben agregarse sin cambiar o borrar las existentes 
