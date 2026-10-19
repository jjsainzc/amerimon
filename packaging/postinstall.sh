#!/bin/sh
set -e

# Secreto JWT: solo root lo lee; systemd lo entrega al servicio como credencial.
mkdir -p /etc/amerimon
if [ ! -s /etc/amerimon/jwt.secret ]; then
    umask 077
    head -c 48 /dev/urandom | base64 -w0 > /etc/amerimon/jwt.secret
fi
chown root:root /etc/amerimon/jwt.secret
chmod 600 /etc/amerimon/jwt.secret

# Credenciales AMI desde manager.conf (primer usuario distinto de [general]); solo si faltan.
ENV_FILE=/etc/amerimon/amerimon.env
MGR_CONF=/etc/asterisk/manager.conf
if [ -f "$MGR_CONF" ]; then
    AMI_USER=$(grep -E '^\s*\[.+\]' "$MGR_CONF" | grep -v '\[general\]' | head -n 1 | tr -d '[] ')
    AMI_SECRET=$(grep -E '^\s*secret\s*=' "$MGR_CONF" | head -n 1 | cut -d'=' -f2 | tr -d ' ')
    if [ -n "$AMI_USER" ] && [ -n "$AMI_SECRET" ]; then
        grep -q '^AMI_USER=' "$ENV_FILE" || printf 'AMI_USER="%s"\n' "$AMI_USER" >> "$ENV_FILE"
        grep -q '^AMI_SECRET=' "$ENV_FILE" || printf 'AMI_SECRET="%s"\n' "$AMI_SECRET" >> "$ENV_FILE"
    else
        echo "Aviso: no se pudo extraer usuario/secret AMI de $MGR_CONF" >&2
    fi
else
    echo "Aviso: no se encuentra $MGR_CONF (AMI sin configurar)" >&2
fi
chown root:root "$ENV_FILE"
chmod 600 "$ENV_FILE"

# allmon.ini propio (lo usa el editor de la web si no hay uno en allmon3/supermon).
OWN_INI=/etc/amerimon/allmon.ini
[ -e "$OWN_INI" ] || : > "$OWN_INI"
chown root:root "$OWN_INI"
chmod 600 "$OWN_INI"

mkdir -p /var/lib/amerimon
chown -R root:root /var/lib/amerimon
chmod 750 /var/lib/amerimon

if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
    systemctl enable amerimon.service
    systemctl restart amerimon.service
    systemctl enable --now amerimon-astdb.timer
fi
