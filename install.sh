#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
if (( EUID == 0 )); then
	SUDO=()
else
	command -v sudo >/dev/null 2>&1 || { echo "Se requiere sudo para instalar amerimon." >&2; exit 1; }
	SUDO=(sudo)
fi

for tool in npm go systemctl getent; do
	command -v "$tool" >/dev/null 2>&1 || { echo "No se encontró $tool." >&2; exit 1; }
done
command -v netstat >/dev/null 2>&1 || { echo "No se encontró netstat; instala net-tools." >&2; exit 1; }
command -v ip >/dev/null 2>&1 || { echo "No se encontró ip; instala iproute2." >&2; exit 1; }
command -v ffmpeg >/dev/null 2>&1 || { echo "No se encontró ffmpeg; instala ffmpeg para convertir los audios." >&2; exit 1; }

cd "$ROOT_DIR"
npm ci
npm run build
CGO_ENABLED=0 go build -trimpath -o dist/amerimon ./cmd/amerimon

"${SUDO[@]}" install -d -m 0755 /etc/amerimon /var/lib/amerimon
"${SUDO[@]}" install -m 0755 dist/amerimon /usr/bin/amerimon

if ! "${SUDO[@]}" test -e /etc/amerimon/amerimon.env; then
	"${SUDO[@]}" install -m 0600 packaging/amerimon.env /etc/amerimon/amerimon.env
fi
if ! "${SUDO[@]}" test -e /etc/amerimon/amerimon.yaml; then
	"${SUDO[@]}" install -m 0644 packaging/amerimon.yaml /etc/amerimon/amerimon.yaml
fi
if ! "${SUDO[@]}" test -s /etc/amerimon/jwt.secret; then
	"${SUDO[@]}" sh -c 'umask 077; head -c 48 /dev/urandom | base64 | tr -d "\n" > /etc/amerimon/jwt.secret'
fi
"${SUDO[@]}" chown root:root /etc/amerimon/amerimon.env /etc/amerimon/amerimon.yaml /etc/amerimon/jwt.secret
"${SUDO[@]}" chmod 0600 /etc/amerimon/amerimon.env /etc/amerimon/jwt.secret
"${SUDO[@]}" chmod 0644 /etc/amerimon/amerimon.yaml

# Keep uploaded WAVs in the sound tree searched by Asterisk.
ASTERISK_DATA_DIR="$("${SUDO[@]}" awk -F '=>' '
	/^[[:space:]]*astdatadir[[:space:]]*=>/ {
		value = $2
		sub(/[;#].*$/, "", value)
		gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
		if (value ~ /^\//) configured = value
	}
	END { print configured }
' /etc/asterisk/asterisk.conf 2>/dev/null || true)"
if [[ "$ASTERISK_DATA_DIR" != /* ]]; then
	ASTERISK_DATA_DIR=/var/lib/asterisk
fi
AUDIO_DIR="$ASTERISK_DATA_DIR/sounds/amerimon"
LEGACY_AUDIO_DIR=/var/lib/asterisk/sounds/amerimon
if getent group asterisk >/dev/null 2>&1; then
	"${SUDO[@]}" install -d -o root -g asterisk -m 2750 "$AUDIO_DIR"
	if [[ -d "$LEGACY_AUDIO_DIR" && "$LEGACY_AUDIO_DIR" != "$AUDIO_DIR" ]]; then
		"${SUDO[@]}" cp -an "$LEGACY_AUDIO_DIR/." "$AUDIO_DIR/"
	fi
else
	echo "Aviso: no existe el grupo asterisk; no se podrán probar reproducciones hasta configurar permisos en $AUDIO_DIR." >&2
fi

if [[ -r /etc/asterisk/manager.conf ]]; then
	AMI_USER="$(grep -E '^\s*\[.+\]' /etc/asterisk/manager.conf | grep -v '\[general\]' | head -n 1 | tr -d '[] ')"
	AMI_SECRET="$(grep -E '^\s*secret\s*=' /etc/asterisk/manager.conf | head -n 1 | cut -d= -f2 | tr -d ' ')"
	if [[ -n "$AMI_USER" && -n "$AMI_SECRET" ]]; then
		if ! "${SUDO[@]}" grep -q '^AMI_USER=' /etc/amerimon/amerimon.env; then
			printf 'AMI_USER="%s"\n' "$AMI_USER" | "${SUDO[@]}" tee -a /etc/amerimon/amerimon.env >/dev/null
		fi
		if ! "${SUDO[@]}" grep -q '^AMI_SECRET=' /etc/amerimon/amerimon.env; then
			printf 'AMI_SECRET="%s"\n' "$AMI_SECRET" | "${SUDO[@]}" tee -a /etc/amerimon/amerimon.env >/dev/null
		fi
	else
		echo "Aviso: no se pudieron leer las credenciales AMI de /etc/asterisk/manager.conf." >&2
	fi
else
	echo "Aviso: no existe /etc/asterisk/manager.conf; AMI queda sin configurar." >&2
fi

if ! "${SUDO[@]}" test -e /etc/amerimon/allmon.ini; then
	"${SUDO[@]}" install -m 0600 /dev/null /etc/amerimon/allmon.ini
fi
"${SUDO[@]}" chown root:root /etc/amerimon/allmon.ini
"${SUDO[@]}" install -d -m 0755 /usr/lib/amerimon /usr/share/amerimon
"${SUDO[@]}" install -m 0644 packaging/listen-live-dialplan.awk /usr/lib/amerimon/listen-live-dialplan.awk
"${SUDO[@]}" install -m 0644 packaging/merge-allstar-public.awk /usr/lib/amerimon/merge-allstar-public.awk
"${SUDO[@]}" install -m 0644 packaging/echolink-peerip.awk /usr/lib/amerimon/echolink-peerip.awk
"${SUDO[@]}" install -m 0644 packaging/echolink-telemetry.awk /usr/lib/amerimon/echolink-telemetry.awk
"${SUDO[@]}" install -m 0644 packaging/configure-pjsip.sh /usr/lib/amerimon/configure-pjsip.sh
"${SUDO[@]}" install -m 0644 packaging/pjsip-network.sh /usr/lib/amerimon/pjsip-network.sh
"${SUDO[@]}" install -m 0644 packaging/amerimon-pjsip-firewall.service /etc/systemd/system/amerimon-pjsip-firewall.service
"${SUDO[@]}" install -m 0644 packaging/merge-pjsip-modules.awk /usr/lib/amerimon/merge-pjsip-modules.awk
"${SUDO[@]}" install -m 0755 packaging/audio_eagi.sh /usr/local/bin/audio_eagi.sh
"${SUDO[@]}" install -m 0755 packaging/astdb.sh /usr/local/bin/astdb.sh
"${SUDO[@]}" install -m 0755 scripts/limpiar_canales_listen_live.sh /usr/local/bin/limpiar_canales_listen_live.sh
"${SUDO[@]}" install -m 0755 packaging/configure-nginx.sh /usr/lib/amerimon/configure-nginx.sh
"${SUDO[@]}" install -m 0644 packaging/apache-amerimon.conf.example packaging/nginx-amerimon.conf.example /usr/share/amerimon/
"${SUDO[@]}" install -m 0644 packaging/pjsip-modules-patch.conf packaging/pjsip-extensions-sip-phones-patch.conf packaging/extensions-allstar-public-patch.conf /usr/share/amerimon/
"${SUDO[@]}" install -m 0644 packaging/amerimon-astdb.service packaging/amerimon-astdb.timer /etc/systemd/system/
"${SUDO[@]}" install -m 0644 packaging/amerimon.service /etc/systemd/system/amerimon.service
"${SUDO[@]}" sh packaging/postinstall.sh
"${SUDO[@]}" systemctl --no-pager --full status amerimon.service
