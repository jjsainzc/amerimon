#!/bin/sh
set -eu

NGINX_BIN=$(command -v nginx) || exit 0
SNIPPET_SOURCE=/usr/share/amerimon/nginx-amerimon.conf.example
SNIPPET_TARGET=/etc/nginx/snippets/amerimon.conf

if [ ! -r "$SNIPPET_SOURCE" ]; then
    echo "Aviso: no se encontró el snippet de Nginx de Amerimon." >&2
    exit 1
fi

NGINX_DUMP=$(mktemp)
ORIGINAL=$(mktemp)
TARGET_TMP=
SNIPPET_ORIGINAL=
SNIPPET_TMP=
SNIPPET_UPDATED=0
cleanup() {
    rm -f "$NGINX_DUMP" "$ORIGINAL"
    [ -z "$TARGET_TMP" ] || rm -f "$TARGET_TMP"
    [ -z "$SNIPPET_ORIGINAL" ] || rm -f "$SNIPPET_ORIGINAL"
    [ -z "$SNIPPET_TMP" ] || rm -f "$SNIPPET_TMP"
}
trap cleanup EXIT HUP INT TERM

if ! "$NGINX_BIN" -T >"$NGINX_DUMP" 2>&1; then
    echo "Aviso: nginx -T falló; no se modificó la configuración de Nginx." >&2
    exit 1
fi

TARGET_INFO=$(awk '
    function is_http_listen(line) {
        return line ~ /^[[:space:]]*listen[[:space:]]/ && line ~ /(^|[[:space]:])80([;[:space:]]|$)/
    }
    /^# configuration file / {
        config = $4
        sub(/:$/, "", config)
        next
    }
    {
        line = $0
        sub(/[[:space:]]*#.*/, "", line)
        if (is_http_listen(line)) {
            if (first == "") first = config "\tfirst"
            if (line ~ /default_server/ && default == "") default = config "\tdefault"
        }
    }
    END {
        if (default != "") print default
        else if (first != "") print first
        else exit 1
    }
' "$NGINX_DUMP") || {
    echo "Aviso: no se encontró un servidor HTTP en la configuración de Nginx." >&2
    exit 1
}

TARGET=$(printf '%s\n' "$TARGET_INFO" | cut -f1)
MODE=$(printf '%s\n' "$TARGET_INFO" | cut -f2)
TARGET=$(readlink -f "$TARGET")
if [ ! -f "$TARGET" ] || [ ! -w "$TARGET" ]; then
    echo "Aviso: no se puede modificar el servidor Nginx seleccionado: $TARGET" >&2
    exit 1
fi

mkdir -p /etc/nginx/snippets
if [ ! -e "$SNIPPET_TARGET" ]; then
    install -m 0644 "$SNIPPET_SOURCE" "$SNIPPET_TARGET"
fi

ACTIVITY_WEBSOCKET_MISSING=0
TERMINAL_WEBSOCKET_MISSING=0
if ! grep -Fq 'location ^~ /amerimon/mon/allstarlink/activity/live' "$SNIPPET_TARGET" &&
    ! grep -Fq 'location /amerimon/mon/allstarlink/activity/live' "$SNIPPET_TARGET"; then
    ACTIVITY_WEBSOCKET_MISSING=1
fi
if ! grep -Fq 'location ^~ /amerimon/mon/terminal' "$SNIPPET_TARGET" &&
    ! grep -Fq 'location /amerimon/mon/terminal' "$SNIPPET_TARGET"; then
    TERMINAL_WEBSOCKET_MISSING=1
fi

if [ "$ACTIVITY_WEBSOCKET_MISSING" -eq 1 ] || [ "$TERMINAL_WEBSOCKET_MISSING" -eq 1 ]; then
    SNIPPET_ORIGINAL=$(mktemp)
    cp -p "$SNIPPET_TARGET" "$SNIPPET_ORIGINAL"
    SNIPPET_TMP=$(mktemp "${SNIPPET_TARGET}.amerimon.XXXXXX")
    cp -p "$SNIPPET_TARGET" "$SNIPPET_TMP"
    if [ "$ACTIVITY_WEBSOCKET_MISSING" -eq 1 ]; then
        cat >>"$SNIPPET_TMP" <<'EOF'

# BEGIN AMERIMON ALLSTARLINK ACTIVITY WEBSOCKET
location ^~ /amerimon/mon/allstarlink/activity/live {
    proxy_pass http://127.0.0.1:8082/mon/allstarlink/activity/live;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_read_timeout 1h;
}
# END AMERIMON ALLSTARLINK ACTIVITY WEBSOCKET
EOF
    fi
    if [ "$TERMINAL_WEBSOCKET_MISSING" -eq 1 ]; then
        cat >>"$SNIPPET_TMP" <<'EOF'

# BEGIN AMERIMON TERMINAL WEBSOCKET
location ^~ /amerimon/mon/terminal {
    proxy_pass http://127.0.0.1:8082/mon/terminal;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_read_timeout 1h;
}
# END AMERIMON TERMINAL WEBSOCKET
EOF
    fi
    chmod --reference="$SNIPPET_TARGET" "$SNIPPET_TMP"
    chown --reference="$SNIPPET_TARGET" "$SNIPPET_TMP"
    mv "$SNIPPET_TMP" "$SNIPPET_TARGET"
    SNIPPET_TMP=
    SNIPPET_UPDATED=1
fi

if ! grep -Fq 'include /etc/nginx/snippets/amerimon.conf;' "$TARGET"; then
    cp -p "$TARGET" "$ORIGINAL"
    TARGET_TMP=$(mktemp "${TARGET}.amerimon.XXXXXX")
    if ! awk -v mode="$MODE" '
        function is_http_listen(line) {
            return line ~ /^[[:space:]]*listen[[:space:]]/ && line ~ /(^|[[:space]:])80([;[:space:]]|$)/
        }
        {
            line = $0
            clean = line
            sub(/[[:space:]]*#.*/, "", clean)
            if (!inserted && is_http_listen(clean)) {
                if (mode == "default" && clean !~ /default_server/) {
                    print line
                    next
                }
                print line
                print "    # BEGIN AMERIMON MANAGED"
                print "    include /etc/nginx/snippets/amerimon.conf;"
                print "    # END AMERIMON MANAGED"
                inserted = 1
                next
            }
            print line
        }
        END { if (!inserted) exit 1 }
    ' "$TARGET" >"$TARGET_TMP"; then
        echo "Aviso: no se encontró la directiva listen HTTP en $TARGET." >&2
        exit 1
    fi
    chmod --reference="$TARGET" "$TARGET_TMP"
    chown --reference="$TARGET" "$TARGET_TMP"
    mv "$TARGET_TMP" "$TARGET"
    TARGET_TMP=
else
    cp -p "$TARGET" "$ORIGINAL"
fi

if ! "$NGINX_BIN" -t; then
    cp -p "$ORIGINAL" "$TARGET"
    if [ "$SNIPPET_UPDATED" -eq 1 ]; then
        cp -p "$SNIPPET_ORIGINAL" "$SNIPPET_TARGET"
    fi
    echo "Aviso: nginx -t falló; se restauró la configuración original." >&2
    exit 1
fi

if systemctl is-active --quiet nginx; then
    systemctl reload nginx || {
        cp -p "$ORIGINAL" "$TARGET"
        if [ "$SNIPPET_UPDATED" -eq 1 ]; then
            cp -p "$SNIPPET_ORIGINAL" "$SNIPPET_TARGET"
        fi
        echo "Aviso: no se pudo recargar Nginx; se restauró la configuración original." >&2
        exit 1
    }
else
    systemctl start nginx || echo "Aviso: no se pudo iniciar Nginx; Amerimon sigue disponible en el puerto 8082." >&2
fi
