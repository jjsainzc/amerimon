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
"${SUDO[@]}" chown root:root /etc/amerimon/amerimon.env /etc/amerimon/amerimon.yaml
"${SUDO[@]}" chmod 0600 /etc/amerimon/amerimon.env
"${SUDO[@]}" chmod 0644 /etc/amerimon/amerimon.yaml
"${SUDO[@]}" install -d -m 0755 /usr/lib/amerimon /usr/share/amerimon/nextion
if compgen -G "public/assets/firmware/AMERINET_Nextion_*" >/dev/null; then
	"${SUDO[@]}" cp -a public/assets/firmware/AMERINET_Nextion_* /usr/share/amerimon/nextion/
fi
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
if ! "${SUDO[@]}" systemctl --no-pager --full status amerimon.service; then
	echo "Aviso: Amerimon no está activo; revisa los errores anteriores y el journal del servicio. La instalación de archivos terminó, pero el servicio requiere atención." >&2
fi
