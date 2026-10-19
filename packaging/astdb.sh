#!/bin/bash

# Directorio de trabajo y archivos (supermon si existe; si no, el de Asterisk, que amerimon lee por defecto)
DIR="${ASTDB_DIR:-/var/www/html/supermon/}"
[ -d "$DIR" ] || DIR="/var/lib/asterisk/"
DB="${DIR}astdb.txt"
PRIVATEFILE="${DIR}privatenodes.txt"

RETRIES=0
PCONTENTS=""
URL="https://allmondb.allstarlink.org/allmondb.php"

# 1. Cargar nodos privados si existen (< 2000)
if [ -f "$PRIVATEFILE" ]; then
    PCONTENTS=$(cat "$PRIVATEFILE")
fi

# 2. Comprobar si es estrictamente un nodo privado (variable de entorno PRIVATE_NODE)
if [ -z "$PRIVATE_NODE" ]; then
    # Si se pasa el argumento 'cron', esperar un tiempo aleatorio entre 0 y 1800 segundos (30 min)
    if [ "$1" = "cron" ]; then
        SECONDS_TO_WAIT=$((RANDOM % 1801))
        echo "Waiting for $SECONDS_TO_WAIT seconds..."
        
        while [ $SECONDS_TO_WAIT -gt 0 ]; do
            if [ $SECONDS_TO_WAIT -gt 60 ]; then
                echo "Sleeping for $SECONDS_TO_WAIT seconds..."
                sleep 60
                SECONDS_TO_WAIT=$((SECONDS_TO_WAIT - 60))
            else
                echo "Sleeping for $SECONDS_TO_WAIT seconds..."
                sleep $SECONDS_TO_WAIT
                SECONDS_TO_WAIT=0
            fi
        done
    fi

    # Bucle de reintentos para descargar la base de datos
    while true; do
        # Descargar usando curl (silencioso y siguiendo redirecciones)
        CONTENTS2=$(curl -sL -m 120 "$URL")
        
        # Obtener el tamaño en bytes de la respuesta
        SIZE=${#CONTENTS2}
        
        if [ "$SIZE" -lt 300000 ]; then
            if [ "$RETRIES" -ge 5 ]; then
                echo "astdb.txt: Retries exceeded!!  $SIZE bytes - Invalid: file too small, bailing out." >&2
                exit 1
            fi
            
            RETRIES=$((RETRIES + 1))
            echo "Retry $RETRIES of 5. Will retry $URL"
            sleep 5
        else
            break
        fi
    done
fi

# Combinar contenido privado y descargado
CONTENTS="${PCONTENTS}${CONTENTS2}"

# Limpiar caracteres no imprimibles
CONTENTS=$(echo "$CONTENTS" | tr -d '\000-\011\013\014\016-\037\177-\377')

# Aplicar sort -nu al resultado final para ordenar numéricamente y eliminar duplicados
CONTENTS=$(echo "$CONTENTS" | sort -nu)

# Guardar los datos en el archivo de destino de forma segura
if ! echo "$CONTENTS" > "$DB"; then
    echo "Cannot write $DB." >&2
    exit 1
fi

SIZE=${#CONTENTS}
echo "Success: astdb.txt $SIZE bytes"