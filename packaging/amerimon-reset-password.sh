#!/bin/sh
# Borra la contraseña del administrador de amerimon; la web pedirá definir una nueva.
# Uso (como root, por ssh): amerimon-reset-password
set -e

if [ "$(id -u)" -ne 0 ]; then
    echo "Ejecute como root: sudo amerimon-reset-password" >&2
    exit 1
fi

ENV_FILE=/etc/amerimon/amerimon.env
if [ -f "$ENV_FILE" ]; then
    set -a
    . "$ENV_FILE"
    set +a
fi
DB="${AMERIMON_DB:-/var/lib/amerimon/amerimon.db}"

/usr/bin/amerimon -reset-admin -db "$DB"
echo "Abra la web y defina la nueva contraseña desde el botón de acceso."
