
# Cambios

- [x] 1. Cada vez que se de click a una pagina poner un cuadrito con icono girando que diga "Cargando ...".

- [x]  2. El clima no se esta ejecutando automaticamente cuando se carga la pagina o cuando cambia de idioma, hay que hacerlo manual.

- [x] 3. Configuracion del pjsip, (modules y extensions si no lo tubieran) para el postinstall al igual que los demas modulos,  en packaging,  pjsip-modules-patch.conf, y para el extensions.conf hay un patch en pjsip-extensions-sip-phones-patch.conf este evita que se haga  crash, el asterisk.

- [X] 4. En la tabla de allstarlink, poner un link en los nodos server, que estan en cabecera de cada tabla, y cuando se le de un click que se seleccione en el combo en la parte de conexión, y una opcion para esconder y mostrar la tabla dejando la cabecera visible.

- [x] 5. Quitar personalizar cabecera y hacer un perfil de usuario admin para configuraciones generales, poner ahi las opciones de la personalizacion de la cabecera.

- [ ] 6. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch

- [ ] 7. En la parte de admin, nodo principal, agregar mantenimiento para esa stanza en whitelist/blacklist de la misma formato que la que existe de permitidos/denegados, el formato en la base es:

```
/whitelist/HC1LNM                             : JJSC
```

`database put <whilelist|blacklist> <callsing> <motivo>`

- [ ] 8. Despues de crear las distribuciones .deb evaluar vaciar el contenido de internal/web/dist se esta acumulando los chunks js y otros files.

- [ ] 9. No es un bugs para los dialogos que modifican informacion, poner por defecto el boton de guardar desactivado y habilitarlo solo si se han hecho cambios.

 
- [ ] 10. Ver ajustes iax users, esconder radio, allstar-sys y allstar-public, si en iax.conf, la stanza  iaxclient, aparece asi `[iaxclient](!)`, entonces buscar en esta linea `#tryinclude custom/iax/iaxrpt-users.conf` (path relativo debajo de `/etc/asterisk`) y poner los users nuevos en esa sección, solo con este formato:
  
```
    [<indicativo>](iaxclient)                ; pull in the config from the [iaxclient] section in
    user = <indicativo>                      ; iax.conf and then add this user's specific information
    secret = Your_Secret_Password_Here ; (user, secret, and callerid). n0call is the USERNAME in
    callerid = "<indicativo en mayúsculas>"                ; their client account configuration.
```
  Si no aparece `[iaxclient](!)`, entonces crear los users como ya esta realizado.

- [ ] 11. Ver la posibilidad de poner una pantalla cliente ssh por el puesto 22, que apunte al ip address del servidor principal.

> **NOTA:** Las actualizaciones no deben quitar las configuraciones personalizadas del cliente, si hay un cambio en las mismas, estas deben agregarse sin cambiar o borrar las existentes 
