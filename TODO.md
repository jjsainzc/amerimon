
# Cambios

- [x] 1. Cada vez que se de click a una pagina poner un cuadrito con icono girando que diga "Cargando ...".

- [x]  2. El clima no se esta ejecutando automaticamente cuando se carga la pagina o cuando cambia de idioma, hay que hacerlo manual.

- [x] 3. Configuracion del pjsip, (modules y extensions si no lo tubieran) para el postinstall al igual que los demas modulos,  en packaging,  pjsip-modules-patch.conf, y para el extensions.conf hay un patch en pjsip-extensions-sip-phones-patch.conf este evita que se haga  crash, el asterisk.

- [X] 4. En la tabla de allstarlink, poner un link en los nodos server, que estan en cabecera de cada tabla, y cuando se le de un click que se seleccione en el combo en la parte de conexión, y una opcion para esconder y mostrar la tabla dejando la cabecera visible.

- [x] 5. Quitar personalizar cabecera y hacer un perfil de usuario admin para configuraciones generales, poner ahi las opciones de la personalizacion de la cabecera.

- [ ] 6. En packaging hay un file llamado  extensions-allstar-public-patch.conf, en el postinstall.sh buscar esa stanza en /etc/asterisk/extensions.conf y aplicar el patch, esa stanza si va a encontrar ahi, solo hay que modificarla y agregarle el chequeo en la base de datos de asterisk que esta en el patch.

- [ ] 7. En la parte de admin, nodo principal, primeramente chequear si no se puede determinar linea que identifica al node (que tiene este formato, por ejemplo, `41777 = radio@127.0.0.1/41777,NONE`), que esta dentro de la stanza `[nodes]`, si es asi emitir una advertencia de que falta la linea correspondiente en el lugar adecuado, etc etc. Posteriormente agregar mantenimiento para esa stanza `[allstar-public]` , CRUD para los indicativos en `whitelist/blacklist` que tiene el chequeo en la base de datos de asterisk, en la misma forma en que se hizo la de permitidos/denegados, el formato en la base para el chequeo en esta stanza es:

```
/whitelist/HC1LNM                             : JJSC
```

`database put <whilelist|blacklist> <callsing> <motivo>`

Organizar estas dos listas de permitidos/denegados y ponerlas en dos tarjetas colapsables porque se vuelven un poco confuso entenderlos bien, esta ultima lista `whitelist/blacklist`,  controla la entrada via indicativo, la otra lista controla la entrada via numero de nodos.

- [x] 8. Despues de crear las distribuciones .deb limpiar el contenido generado de internal/web/dist, conservando .gitkeep. Mantener todos los tests junto al codigo o scripts que prueban, sin borrarlos ni regenerarlos. Revisados: los _test.go ya estan en sus paquetes correspondientes, incluidos los tests de instalacion en packaging.

- [x] 9. En los formularios y dialogos de edicion, Guardar empieza desactivado y se habilita solo con cambios; al restaurar los valores originales vuelve a desactivarse. Aplicado a perfil, favoritos, enlaces externos, Allmon, nodos privados, nodo principal/AMI, IAX/PJSIP y configuracion SSH. Se mantiene la validacion y los cambios tras errores para reintentar; las acciones de conexion y consulta no requieren cambios.

 
- [x] 10. Ver ajustes iax users, esconder las stanzas radio, allstar-sys y allstar-public. Si en iax.conf aparece `[iaxclient](!)`, guardar los users nuevos en el archivo indicado por el `#tryinclude` dentro de ESA MISMA stanza, no en otra. La ruta no es fija: puede ser absoluta o relativa al directorio de iax.conf (normalmente `/etc/asterisk`). Sin esa plantilla, crearlos en iax.conf como hasta ahora. Los comentarios no determinan el destino. Los users del archivo incluido van con este formato:
  
```
    [<indicativo>](iaxclient)                ; pull in the config from the [iaxclient] section in
    user = <indicativo>                      ; iax.conf and then add this user's specific information
    secret = Your_Secret_Password_Here ; (user, secret, and callerid). n0call is the USERNAME in
    callerid = "<indicativo en mayúsculas>"                ; their client account configuration.
```
  Las secciones de usuario heredan de `[iaxclient]` cuando se usa ese archivo.

- [x] 11. Componente que renderize un xterm.js (cliente ssh, que debe tener una pantalla de configuracion, host (por defecto el ipaddress del server principal), puerto y login/passwd o publickey), en la parte de angular en shared hay uno hecho llamado terminal.component para que lo adaptes con el estilo del proyecto, la forma de conexion debe priorizar lo que se configuró previamente, un detalle, para el publickey, este ya previamente debe existir en el server, en el backend dentro de controllers tengo un file que se llama terminal.go, para que lo adaptes a comportarse como lo hace, por ejemplo, un putty ssh client, la intencion es usar el mismo puerto websocket que se esta usando para el listen-live (por supuesto no se puede usar al mismo tiempo, para no tener que abrir otro adicioanl), ya esta instalado el xterm.js de angular, dentro de angular.json agre esto:
``` 
    "styles": [
      "src/styles.scss",
      "node_modules/xterm/css/xterm.css"
    ]
```

- [ ] 12. El pjsip, a traves de un telefono Grandstream, se esta quedando sin audio de recepcion en ocasiones, para resolverlo lo que hago se desconecter y conectar el sip phone, no se si algunas de las operaciones que hicimos le hace tener esta fall, porque anteriormente no pasaba.

  - Ocurre ocasionalmente durante una conexion normal, sin usar mute, desconexion desde la pagina ni Listen Live (la opcion para PJSIP esta oculta). La causa sigue sin confirmar; no cambiar parametros RTP ni configuraciones personalizadas sin evidencia.
  - Cuando vuelva a fallar, **antes de desconectar o reconectar el telefono**, capturar estas salidas para revisar los canales y el trafico RTP:

    ```bash
    sudo asterisk -rx 'pjsip show channelstats'
    sudo asterisk -rx 'core show channels concise'
    ```

  - Revisar tambien la coma adicional antes de `b(...)` en `Dial(Local/${NODE}@rpt-internal,,,b(sip-bridge^addheader^1))` del parche `packaging/pjsip-extensions-sip-phones-patch.conf`. Es una anomalia a investigar, no una causa demostrada de la perdida de audio.
  
- [x] 13. Usemos el mismo websocket que tenemos para la terminal ssh y para el listen-live para determinar cual es la estacion que esta con audio en el momento, para mostrarlo en las tablas, mediante un highlight de la linea qie corresponde a la estacion y que se coloca en la primera posicion de la tabla, o sea si el nodo principal detecta un audio y este esta enlazado a otros server tambien iluminarlos y ponerlos en el tope de cada una, y esto solo se activaría si en el "Actualizar cada" del Allstarlink, se pone "Tiempo Real" (opcion nueva), esta funcionalidad la tiene el Supermon y el Allmon3

- [x] 14. Si se envian nuevas credenciales para la misma localidad en terminal ssh, que se reemplacen por las existentes, mostrar un mensaje para configmar esa opcion, y cuando se haga la conexion, necesito esconder ese box de encabezado para darle el maximo espacio a la terminal ssh.
 
- [x] 15. En la configuracion del pjsip.conf verificar que esto este activo y verificar que el puerto 5060 se active:
```
[transport-udp]
type=transport
protocol=udp    ;udp,tcp,tls,ws,wss,flow
bind=0.0.0.0
``` 

- [ ] 16. Agregar en el perfil del administrador la forma inicial en que se consultan los nodos, si denegado, en tiempo real o por intervalo definido en el combo que ya existe.

- [ ] 17. Actualizar la terminal ssh con el el import del node a `@xterm/xterm` si no es muy problemático el ccambio.

> **NOTA:** Las actualizaciones no deben quitar las configuraciones personalizadas del cliente, si hay un cambio en las mismas, estas deben agregarse sin cambiar o borrar las existentes
