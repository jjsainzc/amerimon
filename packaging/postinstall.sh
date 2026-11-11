#!/bin/sh
set -e

# Run outside a conditional so errexit still stops a failed configuration step.
run_configuration() {
    label=$1
    shift
    set +e
    "$@"
    status=$?
    set -e
    if [ "$status" -ne 0 ]; then
        echo "Aviso: falló la configuración de $label (código $status); se omitió el resto de ese paso y la instalación continúa." >&2
    fi
    return 0
}

configure_jwt() (
set -e
# Secreto JWT: solo root lo lee; systemd lo entrega al servicio como credencial.
mkdir -p /etc/amerimon
if [ ! -s /etc/amerimon/jwt.secret ]; then
    umask 077
    head -c 48 /dev/urandom | base64 -w0 > /etc/amerimon/jwt.secret
fi
chown root:root /etc/amerimon/jwt.secret
chmod 600 /etc/amerimon/jwt.secret
)

configure_ami() (
set -e
mkdir -p /etc/amerimon
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
)

configure_allmon() (
set -e
mkdir -p /etc/amerimon
# allmon.ini propio (lo usa el editor de la web si no hay uno en allmon3/supermon).
OWN_INI=/etc/amerimon/allmon.ini
[ -e "$OWN_INI" ] || : > "$OWN_INI"
chown root:root "$OWN_INI"
chmod 600 "$OWN_INI"
)

configure_state_directory() (
set -e
mkdir -p /var/lib/amerimon
chown -R root:root /var/lib/amerimon
chmod 750 /var/lib/amerimon
)

configure_audio_directory() (
set -e
# Store uploaded WAVs below Asterisk's configured sound tree. The setgid bit
# lets files created by the root-run Amerimon service inherit Asterisk's group.
ASTERISK_DATA_DIR=$(awk -F '=>' '
    /^[[:space:]]*astdatadir[[:space:]]*=>/ {
        value = $2
        sub(/[;#].*$/, "", value)
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        if (value ~ /^\//) configured = value
    }
    END { print configured }
' /etc/asterisk/asterisk.conf 2>/dev/null || true)
case "$ASTERISK_DATA_DIR" in
    /*) ;;
    *) ASTERISK_DATA_DIR=/var/lib/asterisk ;;
esac
AUDIO_DIR="$ASTERISK_DATA_DIR/sounds/amerimon"
LEGACY_AUDIO_DIR=/var/lib/asterisk/sounds/amerimon
if getent group asterisk >/dev/null 2>&1; then
    install -d -o root -g asterisk -m 2750 "$AUDIO_DIR"
    if [ -d "$LEGACY_AUDIO_DIR" ] && [ "$LEGACY_AUDIO_DIR" != "$AUDIO_DIR" ]; then
        cp -an "$LEGACY_AUDIO_DIR/." "$AUDIO_DIR/"
    fi
else
    echo "Aviso: no existe el grupo asterisk; la reproducción de audio requiere configurar permisos en $AUDIO_DIR." >&2
fi
)

configure_terminal_key() (
set -e
mkdir -p /var/lib/amerimon
if [ ! -s /var/lib/amerimon/terminal-credentials.key ]; then
    umask 077
    head -c 32 /dev/urandom > /var/lib/amerimon/terminal-credentials.key
fi
chown root:root /var/lib/amerimon/terminal-credentials.key
chmod 600 /var/lib/amerimon/terminal-credentials.key

)

configure_web_proxy() (
set -e
if [ -d /run/systemd/system ]; then
    # Prefer the active frontend when both web servers are installed.
    if command -v nginx >/dev/null 2>&1 && systemctl is-active --quiet nginx; then
        if [ -x /usr/lib/amerimon/configure-nginx.sh ]; then
            /usr/lib/amerimon/configure-nginx.sh || echo "Aviso: no se pudo configurar Nginx; Amerimon sigue disponible en el puerto 8082." >&2
        fi
    elif command -v a2enmod >/dev/null 2>&1 && [ -f /usr/share/amerimon/apache-amerimon.conf.example ]; then
        APACHE_CONF=/etc/apache2/conf-available/amerimon.conf
        APACHE_WAS_ENABLED=0
        APACHE_PATCH_BACKUP=
        APACHE_PATCH_TMP=
        APACHE_PATCHED=0
        [ -e /etc/apache2/conf-enabled/amerimon.conf ] && APACHE_WAS_ENABLED=1
        if [ ! -e "$APACHE_CONF" ]; then
            install -D -m 0644 /usr/share/amerimon/apache-amerimon.conf.example "$APACHE_CONF"
        fi
        APACHE_NEED_ACTIVITY_PROXY=1
        APACHE_NEED_ACTIVITY_PROXY_REVERSE=1
        APACHE_NEED_TERMINAL_PROXY=1
        APACHE_NEED_TERMINAL_PROXY_REVERSE=1
        grep -Eq '^[[:space:]]*ProxyPass[[:space:]]+/amerimon/mon/allstarlink/activity/live[[:space:]]+ws://' "$APACHE_CONF" && APACHE_NEED_ACTIVITY_PROXY=0
        grep -Eq '^[[:space:]]*ProxyPassReverse[[:space:]]+/amerimon/mon/allstarlink/activity/live[[:space:]]+ws://' "$APACHE_CONF" && APACHE_NEED_ACTIVITY_PROXY_REVERSE=0
        grep -Eq '^[[:space:]]*ProxyPass[[:space:]]+/amerimon/mon/terminal[[:space:]]+ws://' "$APACHE_CONF" && APACHE_NEED_TERMINAL_PROXY=0
        grep -Eq '^[[:space:]]*ProxyPassReverse[[:space:]]+/amerimon/mon/terminal[[:space:]]+ws://' "$APACHE_CONF" && APACHE_NEED_TERMINAL_PROXY_REVERSE=0
        if [ "$APACHE_NEED_ACTIVITY_PROXY" -eq 1 ] || [ "$APACHE_NEED_ACTIVITY_PROXY_REVERSE" -eq 1 ] ||
            [ "$APACHE_NEED_TERMINAL_PROXY" -eq 1 ] || [ "$APACHE_NEED_TERMINAL_PROXY_REVERSE" -eq 1 ]; then
            APACHE_PATCH_BACKUP=$(mktemp)
            cp -p "$APACHE_CONF" "$APACHE_PATCH_BACKUP"
            APACHE_PATCH_TMP=$(mktemp "${APACHE_CONF}.amerimon.XXXXXX")
            awk \
                -v add_activity_proxy="$APACHE_NEED_ACTIVITY_PROXY" \
                -v add_activity_reverse="$APACHE_NEED_ACTIVITY_PROXY_REVERSE" \
                -v add_terminal_proxy="$APACHE_NEED_TERMINAL_PROXY" \
                -v add_terminal_reverse="$APACHE_NEED_TERMINAL_PROXY_REVERSE" '
                function websocket_routes() {
                    if (add_activity_proxy) print "ProxyPass        /amerimon/mon/allstarlink/activity/live ws://127.0.0.1:8082/mon/allstarlink/activity/live"
                    if (add_activity_reverse) print "ProxyPassReverse /amerimon/mon/allstarlink/activity/live ws://127.0.0.1:8082/mon/allstarlink/activity/live"
                    if (add_terminal_proxy) print "ProxyPass        /amerimon/mon/terminal ws://127.0.0.1:8082/mon/terminal"
                    if (add_terminal_reverse) print "ProxyPassReverse /amerimon/mon/terminal ws://127.0.0.1:8082/mon/terminal"
                }
                /^[[:space:]]*ProxyPass[[:space:]]/ && !inserted {
                    websocket_routes()
                    inserted = 1
                }
                { print }
                END {
                    if (!inserted) websocket_routes()
                }
            ' "$APACHE_CONF" >"$APACHE_PATCH_TMP"
            chmod --reference="$APACHE_CONF" "$APACHE_PATCH_TMP"
            chown --reference="$APACHE_CONF" "$APACHE_PATCH_TMP"
            mv "$APACHE_PATCH_TMP" "$APACHE_CONF"
            APACHE_PATCH_TMP=
            APACHE_PATCHED=1
        fi
        if a2enmod proxy proxy_http proxy_wstunnel headers alias >/dev/null && \
            a2enconf amerimon >/dev/null && apache2ctl configtest; then
            if systemctl is-active --quiet apache2; then
                systemctl reload apache2 || echo "Aviso: no se pudo recargar Apache; Amerimon sigue disponible en el puerto 8082." >&2
            else
                systemctl start apache2 || echo "Aviso: no se pudo iniciar Apache; Amerimon sigue disponible en el puerto 8082." >&2
            fi
        else
            if [ "$APACHE_PATCHED" -eq 1 ]; then
                cp -p "$APACHE_PATCH_BACKUP" "$APACHE_CONF"
            fi
            if [ "$APACHE_WAS_ENABLED" -eq 0 ]; then
                a2disconf amerimon >/dev/null 2>&1 || true
            fi
            echo "Aviso: la configuración de Apache no pasó configtest; se dejó deshabilitada para proteger el sitio existente." >&2
        fi
        [ -z "$APACHE_PATCH_BACKUP" ] || rm -f "$APACHE_PATCH_BACKUP"
    elif command -v nginx >/dev/null 2>&1 && [ -x /usr/lib/amerimon/configure-nginx.sh ]; then
        /usr/lib/amerimon/configure-nginx.sh || echo "Aviso: no se pudo configurar Nginx; Amerimon sigue disponible en el puerto 8082." >&2
    fi
fi
)

# Listen Live and inbound audio control need Asterisk modules. Keep backups and
# preserve existing Asterisk configuration; no Asterisk restart is performed.
configure_allstar_public() (
set -e
    EXTENSIONS_CONF="$AST_DIR/extensions.conf"
    ALLSTAR_PUBLIC_PATCH=/usr/share/amerimon/extensions-allstar-public-patch.conf
    ALLSTAR_PUBLIC_MERGER=/usr/lib/amerimon/merge-allstar-public.awk
    if [ -f "$EXTENSIONS_CONF" ]; then
        if [ -r "$ALLSTAR_PUBLIC_PATCH" ] && [ -r "$ALLSTAR_PUBLIC_MERGER" ]; then
            merged=$(mktemp "${EXTENSIONS_CONF}.allstar-public.XXXXXX")
            if awk -v patch="$ALLSTAR_PUBLIC_PATCH" -f "$ALLSTAR_PUBLIC_MERGER" "$EXTENSIONS_CONF" > "$merged"; then
                if ! cmp -s "$EXTENSIONS_CONF" "$merged"; then
                    [ -e "$EXTENSIONS_CONF.bak-allstar-public" ] || cp -a "$EXTENSIONS_CONF" "$EXTENSIONS_CONF.bak-allstar-public"
                    chmod --reference="$EXTENSIONS_CONF" "$merged"
                    chown --reference="$EXTENSIONS_CONF" "$merged"
                    mv "$merged" "$EXTENSIONS_CONF"
                else
                    rm -f "$merged"
                fi
            else
                status=$?
                rm -f "$merged"
                if [ "$status" -eq 3 ]; then
                    echo "Aviso: no se encontró [allstar-public] en $EXTENSIONS_CONF; no se aplicó el control de indicativos." >&2
                else
                    echo "Error: no se pudo aplicar el parche [allstar-public] a $EXTENSIONS_CONF." >&2
                    exit 1
                fi
            fi
        else
            echo "Aviso: falta el parche o el aplicador de [allstar-public]; no se modificó $EXTENSIONS_CONF." >&2
        fi
    else
        echo "Aviso: no existe $EXTENSIONS_CONF; no se aplicó el control [allstar-public]." >&2
    fi

)

configure_echolink_peerip() (
set -e
    ECHOLINK_PEERIP_PATCH=/usr/lib/amerimon/echolink-peerip.awk
    if [ -f "$EXTENSIONS_CONF" ] && [ -r "$ECHOLINK_PEERIP_PATCH" ]; then
        merged=$(mktemp "${EXTENSIONS_CONF}.echolink-peerip.XXXXXX")
        if awk -f "$ECHOLINK_PEERIP_PATCH" "$EXTENSIONS_CONF" > "$merged"; then
            if ! cmp -s "$EXTENSIONS_CONF" "$merged"; then
                [ -e "$EXTENSIONS_CONF.bak-echolink-peerip" ] || cp -a "$EXTENSIONS_CONF" "$EXTENSIONS_CONF.bak-echolink-peerip"
                chmod --reference="$EXTENSIONS_CONF" "$merged"
                chown --reference="$EXTENSIONS_CONF" "$merged"
                mv "$merged" "$EXTENSIONS_CONF"
            else
                rm -f "$merged"
            fi
        else
            status=$?
            rm -f "$merged"
            if [ "$status" -eq 3 ]; then
                echo "Aviso: no se encontró [radio-secure]; no se aplicó el parche peerip de EchoLink." >&2
            else
                echo "Error: no se pudo aplicar el parche peerip de EchoLink." >&2
                exit 1
            fi
        fi
    else
        echo "Aviso: falta extensions.conf o el parche peerip de EchoLink; no se aplicó." >&2
    fi

)

configure_echolink_telemetry() (
set -e
    ECHOLINK_CONF="$AST_DIR/echolink.conf"
    RPT_CONF="$AST_DIR/rpt.conf"
    ECHOLINK_TELEMETRY_PATCH=/usr/lib/amerimon/echolink-telemetry.awk
    if [ -f "$ECHOLINK_CONF" ] && [ -f "$RPT_CONF" ] && [ -r "$ECHOLINK_TELEMETRY_PATCH" ]; then
        merged=$(mktemp "${RPT_CONF}.echolink-telemetry.XXXXXX")
        if awk -f "$ECHOLINK_TELEMETRY_PATCH" "$ECHOLINK_CONF" "$RPT_CONF" > "$merged"; then
            if ! cmp -s "$RPT_CONF" "$merged"; then
                [ -e "$RPT_CONF.bak-echolink-telemetry" ] || cp -a "$RPT_CONF" "$RPT_CONF.bak-echolink-telemetry"
                chmod --reference="$RPT_CONF" "$merged"
                chown --reference="$RPT_CONF" "$merged"
                mv "$merged" "$RPT_CONF"
                echo "EchoLink: se configuró echolinkdefault = 1; también habilita otra telemetría hacia EchoLink."
                if command -v asterisk >/dev/null 2>&1 && asterisk -rx 'core show version' >/dev/null 2>&1; then
                    asterisk -rx 'module reload app_rpt.so' || echo "Aviso: no se pudo recargar app_rpt.so; recárgalo manualmente." >&2
                else
                    echo "Aviso: Asterisk no está disponible; recarga app_rpt.so cuando esté activo." >&2
                fi
                echo "EchoLink: desconecta y reconecta los enlaces para aplicar el modo de telemetría."
            else
                rm -f "$merged"
            fi
        else
            status=$?
            rm -f "$merged"
            if [ "$status" -eq 3 ]; then
                echo "Aviso: no se encontró la plantilla [node-main] ni un astnode con sección en rpt.conf; no se modificó rpt.conf." >&2
            else
                echo "Error: no se pudo aplicar el parche de telemetría EchoLink; rpt.conf no se modificó." >&2
                exit 1
            fi
        fi
    else
        echo "Aviso: falta echolink.conf, rpt.conf o el parche de telemetría EchoLink; no se aplicó." >&2
    fi

)

configure_pjsip() (
set -e
    if [ -r /usr/lib/amerimon/configure-pjsip.sh ]; then
        sh /usr/lib/amerimon/configure-pjsip.sh
    else
        echo "Error: no se encuentra /usr/lib/amerimon/configure-pjsip.sh; no se configuró PJSIP." >&2
        exit 1
    fi

)

configure_audio_modules() (
set -e
    MODULES_CONF="$AST_DIR/modules.conf"
    if [ -f "$MODULES_CONF" ]; then
        [ -e "$MODULES_CONF.bak-listen-live" ] || cp -a "$MODULES_CONF" "$MODULES_CONF.bak-listen-live"
        AVAILABLE_MODULES=
        for module in $MODULES; do
            module_found=0
            for module_file in /usr/lib/*/asterisk/modules/"$module" /usr/lib/asterisk/modules/"$module"; do
                if [ -f "$module_file" ]; then
                    module_found=1
                    break
                fi
            done
            if [ "$module_found" -eq 0 ]; then
                echo "Aviso: módulo Asterisk no instalado: $module" >&2
                continue
            fi
                AVAILABLE_MODULES="$AVAILABLE_MODULES $module"
        done

        if [ -n "$AVAILABLE_MODULES" ]; then
            sed -i '/^; BEGIN AMERIMON LISTEN LIVE MODULES$/,/^; END AMERIMON LISTEN LIVE MODULES$/d' "$MODULES_CONF"
            for module in $AVAILABLE_MODULES; do
                module_pattern=$(printf '%s' "$module" | sed 's/\./\\./g')
                sed -i -E "/^[[:space:]]*(load|noload)[[:space:]]*(=>|=)[[:space:]]*$module_pattern([[:space:]]*(;.*)?)$/d" "$MODULES_CONF"
            done
            {
                printf '; BEGIN AMERIMON LISTEN LIVE MODULES\n'
                for module in $AVAILABLE_MODULES; do
                    printf 'load => %s\n' "$module"
                done
                printf '; END AMERIMON LISTEN LIVE MODULES\n'
            } >> "$MODULES_CONF"
        fi
    else
        echo "Aviso: no existe $MODULES_CONF; no se configuraron módulos para Listen Live y control de audio." >&2
    fi

)

configure_listen_live() (
set -e
    EXTENSIONS_CONF="$AST_DIR/extensions.conf"
    if [ -f "$EXTENSIONS_CONF" ]; then
        [ -e "$EXTENSIONS_CONF.bak-listen-live" ] || cp -a "$EXTENSIONS_CONF" "$EXTENSIONS_CONF.bak-listen-live"

        migrated=$(mktemp "${EXTENSIONS_CONF}.amerimon.XXXXXX")
        if awk -f /usr/lib/amerimon/listen-live-dialplan.awk "$EXTENSIONS_CONF" > "$migrated"; then
            if ! cmp -s "$EXTENSIONS_CONF" "$migrated"; then
                cp -a "$EXTENSIONS_CONF" "$EXTENSIONS_CONF.bak-listen-live-rxchannel"
                cat "$migrated" > "$EXTENSIONS_CONF"
            fi
            rm -f "$migrated"
        else
            rm -f "$migrated"
            echo "Error: no se pudo migrar el dialplan Listen Live." >&2
            exit 1
        fi

        if ! grep -qE '^[[:space:]]*\[amerimon-listen-node\][[:space:]]*(;.*)?$' "$EXTENSIONS_CONF"; then
            cat >> "$EXTENSIONS_CONF" <<'EOF'

; --- Amerimon: escucha del rxchannel efectivo de rpt.conf ---
[amerimon-listen-node]
exten => h,1,NoOp(Listen Live node channel ended)
 same => n,Hangup()
exten => _X.,1,NoOp(Listen Live rxchannel nodo ${EXTEN})
 same => n,Set(AMERIMON_RPT_CONFIG=${IF($["${AMERIMON_RPT_CONFIG}" = ""]?rpt.conf:${AMERIMON_RPT_CONFIG})})
 same => n,Set(AMERIMON_RX=${AST_CONFIG(${AMERIMON_RPT_CONFIG},${EXTEN},rxchannel,-1)})
 same => n,GotoIf($["${AMERIMON_RX}" = ""]?missing)
 same => n,Set(AMERIMON_TECH=${CUT(AMERIMON_RX,/,1)})
 same => n,ExecIf($["${TOLOWER(${AMERIMON_TECH})}" = "local"]?Set(AMERIMON_RX=${CUT(AMERIMON_RX,/,1-2)}))
 same => n,ExecIf($["${TOLOWER(${AMERIMON_TECH})}" = "local" & ${REGEX("@" ${AMERIMON_RX})} = 0]?Set(AMERIMON_RX=${AMERIMON_RX}@default))
 same => n,Verbose(1,Amerimon Listen Live nodo ${EXTEN} rxchannel ${AMERIMON_RX})
 same => n,Answer()
 same => n,ChanSpy(${AMERIMON_RX},q)
 same => n,Hangup()
 same => n(missing),Log(ERROR,Amerimon Listen Live sin rxchannel para nodo ${EXTEN} en ${AMERIMON_RPT_CONFIG})
 same => n,Hangup()
EOF
        fi

        if ! grep -qE '^[[:space:]]*\[amerimon-record-node\][[:space:]]*(;.*)?$' "$EXTENSIONS_CONF"; then
            cat >> "$EXTENSIONS_CONF" <<'EOF'

; --- Amerimon: mezcla del nodo y enlaces, solo escucha para grabacion ---
[amerimon-record-node]
exten => h,1,NoOp(Recording node channel ended)
 same => n,Hangup()
exten => _X.,1,NoOp(Recording node mix ${EXTEN})
 same => n,Answer()
 same => n,Rpt(${EXTEN},Pm)
 same => n,Hangup()
EOF
        fi

        if ! grep -qE '^[[:space:]]*\[listen-live-spy\][[:space:]]*$' "$EXTENSIONS_CONF"; then
            cat >> "$EXTENSIONS_CONF" <<'EOF'

; --- Amerimon Listen Live: Local;1 espía y Local;2 entrega audio EAGI ---
[listen-live-spy]
exten => h,1,NoOp(Listen Live spy channel ended)
 same => n,Hangup()
exten => _.,1,NoOp(Espiando PJSIP/${EXTEN})
 same => n,Answer()
 same => n,ChanSpy(PJSIP/${EXTEN},q)
 same => n,Hangup()
exten => nodo,1,Goto(amerimon-listen-node,${NODE},1)
EOF
        fi

        if ! awk '
            /^[[:space:]]*\[listen-live-spy\][[:space:]]*(;.*)?$/ { in_context = 1; next }
            /^\[/ { if (in_context) exit; in_context = 0 }
            in_context && /^[[:space:]]*exten[[:space:]]*=>[[:space:]]*nodo[[:space:]]*,/ { found = 1 }
            END { exit !found }
        ' "$EXTENSIONS_CONF"; then
            sed -i '/^[[:space:]]*\[listen-live-spy\][[:space:]]*$/a exten => nodo,1,Goto(amerimon-listen-node,${NODE},1)' "$EXTENSIONS_CONF"
        fi

        if ! grep -qE '^[[:space:]]*\[listen-live-eagi\][[:space:]]*$' "$EXTENSIONS_CONF"; then
            cat >> "$EXTENSIONS_CONF" <<EOF

[listen-live-eagi]
exten => h,1,NoOp(Listen Live EAGI channel ended)
 same => n,Hangup()
exten => _.,1,NoOp(EAGI listen-live UUID \${EXTEN})
 same => n,Answer()
 same => n,EAGI(/usr/local/bin/audio_eagi.sh,\${EXTEN},${LISTENLIVE_PORT:-8086})
 same => n,Hangup()
EOF
        elif ! awk '
            /^\[listen-live-eagi\][[:space:]]*$/ { in_context = 1; next }
            /^\[/ { if (in_context) exit; in_context = 0 }
            in_context && /EAGI\(/ { found = 1 }
            END { exit !found }
        ' "$EXTENSIONS_CONF"; then
            echo "Aviso: existe [listen-live-eagi] pero no contiene EAGI(); no se sobrescribió." >&2
        fi

        if command -v asterisk >/dev/null 2>&1 && asterisk -rx 'core show version' >/dev/null 2>&1; then
            for module in $MODULES; do
                if [ "$module" = res_mutestream.so ]; then
                    if ! asterisk -rx 'module show like res_mutestream' 2>/dev/null | grep -q '^res_mutestream\.so[[:space:]]'; then
                        if ! asterisk -rx 'module load res_mutestream.so' >/dev/null 2>&1 ||
                           ! asterisk -rx 'module show like res_mutestream' 2>/dev/null | grep -q '^res_mutestream\.so[[:space:]]'; then
                            echo "Aviso: no se pudo habilitar res_mutestream.so; el control de audio permanecerá deshabilitado." >&2
                        fi
                    fi
                else
                    asterisk -rx "module load $module" >/dev/null 2>&1 || true
                fi
            done
            asterisk -rx 'dialplan reload' >/dev/null 2>&1 || echo "Aviso: no se pudo recargar el dialplan; los cambios se aplicarán al reiniciar Asterisk." >&2
        fi
    else
        echo "Aviso: no existe $EXTENSIONS_CONF; no se configuró el dialplan de Listen Live." >&2
    fi
)

run_configuration "secreto JWT" configure_jwt
run_configuration "credenciales AMI" configure_ami
run_configuration "allmon.ini" configure_allmon
run_configuration "directorio de estado" configure_state_directory
run_configuration "directorio de audio" configure_audio_directory
run_configuration "clave de terminal" configure_terminal_key
if [ -d /run/systemd/system ]; then
    run_configuration "recarga de systemd" systemctl daemon-reload
    run_configuration "habilitación de Amerimon" systemctl enable amerimon.service
    run_configuration "reinicio de Amerimon" systemctl restart amerimon.service
    run_configuration "temporizador astdb" systemctl enable --now amerimon-astdb.timer
fi
run_configuration "proxy web" configure_web_proxy

AST_DIR=/etc/asterisk
EXTENSIONS_CONF="$AST_DIR/extensions.conf"
MODULES="res_speech.so res_agi.so app_chanspy.so app_mixmonitor.so app_verbose.so func_config.so func_strings.so func_logic.so func_cut.so res_mutestream.so"
if [ -d "$AST_DIR" ]; then
    run_configuration "allstar-public" configure_allstar_public
    run_configuration "EchoLink peerip" configure_echolink_peerip
    run_configuration "telemetría EchoLink" configure_echolink_telemetry
    run_configuration "PJSIP" configure_pjsip
    run_configuration "módulos de audio" configure_audio_modules
    run_configuration "dialplan Listen Live" configure_listen_live
fi

exit 0
