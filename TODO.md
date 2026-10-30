
# Cambios

- [x] 1. Cada vez que se de click a una pagina poner un cuadrito con icono girando que diga "Cargando ...".

- [x]  2. El clima no se esta ejecutando automaticamente cuando se carga la pagina o cuando cambia de idioma, hay que hacerlo manual.

- [x] 3. Configuracion del pjsip, (modules y extensions si no lo tubieran) para el postinstall al igual que los demas modulos,  en packaging,  pjsip-modules-patch.conf, y para el extensions.conf hay un patch en pjsip-extensions-sip-phones-patch.conf este evita que se haga  crash, el asterisk.

- [X] 4. En la tabla de allstarlink, poner un link en los nodos server, que estan en cabecera de cada tabla, y cuando se le de un click que se seleccione en el combo en la parte de conexión, y una opcion para esconder y mostrar la tabla dejando la cabecera visible.

- [x] 5. Quitar personalizar cabecera y hacer un perfil de usuario admin para configuraciones generales, poner ahi las opciones de la personalizacion de la cabecera.

- [ ] 6. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch, esa stanza se va a estar ahi, solo hay que modificarla y agregarle el chequeo en la base de datos de asterisk que esta en el patch.

- [ ] 7. En la parte de admin, nodo principal, primeramente chequear si no se puede determinar linea que identifica al node (que tiene este formato, por ejemplo, `41777 = radio@127.0.0.1/41777,NONE`), que esta dentro de la stanza `[nodes]`, si es asi emitir una advertencia de que falta la linea correspondiente en el lugar adecuado, etc etc. Y agregar mantenimiento para esa stanza `[allstar-public]` , que tiene el chequeo en la base de datos de asteris, `whitelist/blacklist`,  en la misma forma en que se hizo la de permitidos/denegados, el formato en la base para el chequeo en esta stanza es:

```
/whitelist/HC1LNM                             : JJSC
```

`database put <whilelist|blacklist> <callsing> <motivo>`

- [ ] 8. Despues de crear las distribuciones .deb evaluar vaciar el contenido de internal/web/dist se esta acumulando los chunks js y otros files, mover los _test.go a los lugares adecuados, estan en varios lugares.

- [ ] 9. No es un bugs, pero para los dialogos que modifican informacion, poner por defecto el boton de guardar desactivado y habilitarlo solo si se han hecho cambios.

 
- [ ] 10. Ver ajustes iax users, esconder las stanzas radio, allstar-sys y allstar-public, si en iax.conf, la stanza  iaxclient, aparece de esta forma `[iaxclient](!)` (quiere decir que es una plantilla), entonces buscar la linea `#tryinclude custom/iax/iaxrpt-users.conf` que aparece mas debao, y que es un path relativo a `/etc/asterisk`) y poner los users nuevos en esa sección, solo con este formato:
  
```
    [<indicativo>](iaxclient)                ; pull in the config from the [iaxclient] section in
    user = <indicativo>                      ; iax.conf and then add this user's specific information
    secret = Your_Secret_Password_Here ; (user, secret, and callerid). n0call is the USERNAME in
    callerid = "<indicativo en mayúsculas>"                ; their client account configuration.
```
  Si no aparece `[iaxclient](!)`, entonces crear los users como ya esta realizado.

- [x] 11. Componente que renderize un xterm.js (cliente ssh, que debe tener una pantalla de configuracion, host (por defecto el ipaddress del server principal), puerto y login/passwd o publickey), en la parte de angular en shared hay uno hecho llamado terminal.component para que lo adaptes con el estilo del proyecto, la forma de conexion debe priorizar lo que se configuró previamente, un detalle, para el publickey, este ya previamente debe existir en el server, en el backend dentro de controllers tengo un file que se llama terminal.go, para que lo adaptes a comportarse como lo hace, por ejemplo, un putty ssh client, la intencion es usar el mismo puerto websocket que se esta usando para el listen-live (por supuesto no se puede usar al mismo tiempo, para no tener que abrir otro adicioanl), ya esta instalado el xterm.js de angular, dentro de angular.json agre esto:
``` 
    "styles": [
      "src/styles.scss",
      "node_modules/xterm/css/xterm.css"
    ]
```

- [ ] 12. El pjsip, a traves de un telefono Grandstream, se esta quedando sin audio de recepcion en ocasiones, para resolverlo lo que hago se desconecter y conectar el sip phone, no se si algunas de las operaciones que hicimos le hace tener esta fall, porque anteriormente no pasaba.

> **NOTA:** Las actualizaciones no deben quitar las configuraciones personalizadas del cliente, si hay un cambio en las mismas, estas deben agregarse sin cambiar o borrar las existentes 

