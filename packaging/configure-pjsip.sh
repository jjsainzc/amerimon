#!/bin/sh
set -e

AST_DIR=${AST_DIR:-/etc/asterisk}
PJSIP_MODULES_PATCH=${PJSIP_MODULES_PATCH:-/usr/share/amerimon/pjsip-modules-patch.conf}
PJSIP_EXTENSIONS_PATCH=${PJSIP_EXTENSIONS_PATCH:-/usr/share/amerimon/pjsip-extensions-sip-phones-patch.conf}
PJSIP_MODULES_MERGER=${PJSIP_MODULES_MERGER:-/usr/lib/amerimon/merge-pjsip-modules.awk}

[ -d "$AST_DIR" ] || exit 0

for resource in "$PJSIP_MODULES_PATCH" "$PJSIP_EXTENSIONS_PATCH" "$PJSIP_MODULES_MERGER"; do
    if [ ! -r "$resource" ]; then
        echo "Error: no se puede leer el recurso de configuración PJSIP $resource." >&2
        exit 1
    fi
done

MODULES_CONF="$AST_DIR/modules.conf"
modules_created=0
if [ ! -e "$MODULES_CONF" ]; then
    install -m 0640 /dev/null "$MODULES_CONF"
    printf '[modules]\nautoload=yes\n' > "$MODULES_CONF"
    modules_created=1
fi

merged_modules=$(mktemp "${MODULES_CONF}.pjsip-merged.XXXXXX")
modules_source=$(mktemp "${MODULES_CONF}.pjsip-source.XXXXXX")
load_errors=$(mktemp "${MODULES_CONF}.pjsip-errors.XXXXXX")
trap 'rm -f "$merged_modules" "$modules_source" "$load_errors"' EXIT HUP INT TERM
AVAILABLE_MODULES=

while IFS= read -r line || [ -n "$line" ]; do
    module=$(printf '%s\n' "$line" | sed -nE 's/^[[:space:]]*load[[:space:]]*(=>|=)[[:space:]]*([^[:space:];]+).*/\2/p')
    if [ -n "$module" ]; then
        AVAILABLE_MODULES="$AVAILABLE_MODULES $module"
    fi
done < "$PJSIP_MODULES_PATCH"

# Elimina únicamente la línea de carga gestionada por el instalador anterior. Conserva cualquier configuración explícita
# del administrador para la compatibilidad con PUBLISH entre servidores.
awk '
    !/^[[:space:]]*load[[:space:]]*(=>|=)[[:space:]]*res_pjsip_publish_asterisk\.so[[:space:]]*;[[:space:]]*PJSIP Asterisk Event PUBLISH Support[[:space:]]*$/
' "$MODULES_CONF" > "$modules_source"

if awk -v config="$modules_source" -v patch="$PJSIP_MODULES_PATCH" -f "$PJSIP_MODULES_MERGER" "$modules_source" > "$merged_modules"; then
    if ! cmp -s "$MODULES_CONF" "$merged_modules"; then
        if [ "$modules_created" -eq 0 ] && [ ! -e "$MODULES_CONF.bak-pjsip" ]; then
            cp -a "$MODULES_CONF" "$MODULES_CONF.bak-pjsip"
        fi
        cat "$merged_modules" > "$MODULES_CONF"
    fi
else
    echo "Error: no se pudo integrar el parche de módulos PJSIP." >&2
    exit 1
fi

EXTENSIONS_CONF="$AST_DIR/extensions.conf"
extensions_created=0
if [ ! -e "$EXTENSIONS_CONF" ]; then
    install -m 0640 /dev/null "$EXTENSIONS_CONF"
    extensions_created=1
fi

for context in sip-phones rpt-internal; do
    context_patch=$(mktemp "${EXTENSIONS_CONF}.pjsip-context.XXXXXX")
    if ! awk -v context="$context" '
        /^[[:space:]]*\[/ {
            if (in_context) {
                exit
            }
            if ($0 ~ "^[[:space:]]*\\[" context "\\][[:space:]]*(;.*)?$") {
                in_context = 1
            }
        }
        in_context { print }
    ' "$PJSIP_EXTENSIONS_PATCH" > "$context_patch" || [ ! -s "$context_patch" ]; then
        rm -f "$context_patch"
        echo "Error: no se encontró [$context] en $PJSIP_EXTENSIONS_PATCH." >&2
        exit 1
    fi

    if grep -qE "^[[:space:]]*\\[$context\\][[:space:]]*(;.*)?$" "$EXTENSIONS_CONF"; then
        if awk -v context="$context" '
            /^[[:space:]]*\[/ {
                in_context = ($0 ~ "^[[:space:]]*\\[" context "\\][[:space:]]*(;.*)?$")
                next
            }
            in_context && /^[[:space:]]*exten[[:space:]]*=>[[:space:]]*_X[.],/ {
                found = 1
            }
            END { exit !found }
        ' "$EXTENSIONS_CONF"; then
            case "$context" in
                sip-phones) expected_route='exten => _X.,1,Ringing()' ;;
                rpt-internal) expected_route='exten => _X.,1,Rpt(' ;;
            esac
            if ! awk -v context="$context" -v expected_route="$expected_route" '
                /^[[:space:]]*\[/ {
                    in_context = ($0 ~ "^[[:space:]]*\\[" context "\\][[:space:]]*(;.*)?$")
                    next
                }
                in_context && index($0, expected_route) > 0 {
                    found = 1
                }
                END { exit !found }
            ' "$EXTENSIONS_CONF"; then
                echo "Aviso: [$context] ya define _X.; se conserva esa ruta personalizada y no se agrega la ruta PJSIP para evitar prioridades duplicadas." >&2
            fi
            rm -f "$context_patch"
            continue
        fi

        body_patch=$(mktemp "${EXTENSIONS_CONF}.pjsip-body.XXXXXX")
        merged_extensions=$(mktemp "${EXTENSIONS_CONF}.pjsip-merged.XXXXXX")
        awk -v context="$context" '
            /^[[:space:]]*\[/ {
                if (in_context) {
                    exit
                }
                if ($0 ~ "^[[:space:]]*\\[" context "\\][[:space:]]*(;.*)?$") {
                    in_context = 1
                    next
                }
            }
            in_context { print }
        ' "$context_patch" > "$body_patch"
        if [ ! -s "$body_patch" ]; then
            rm -f "$context_patch" "$body_patch" "$merged_extensions"
            echo "Error: el contexto [$context] no contiene líneas para integrar." >&2
            exit 1
        fi

        awk -v context="$context" -v body="$body_patch" '
            BEGIN {
                while ((getline line < body) > 0) {
                    body_lines[++body_count] = line
                }
                close(body)
            }
            function append_body() {
                print ""
                for (i = 1; i <= body_count; i++) {
                    print body_lines[i]
                }
            }
            {
                if ($0 ~ "^[[:space:]]*\\[" context "\\][[:space:]]*(;.*)?$") {
                    in_context = 1
                } else if ($0 ~ /^[[:space:]]*\[/ && in_context) {
                    append_body()
                    in_context = 0
                }
                print
            }
            END {
                if (in_context) {
                    append_body()
                }
            }
        ' "$EXTENSIONS_CONF" > "$merged_extensions"
        if ! cmp -s "$EXTENSIONS_CONF" "$merged_extensions"; then
            if [ "$extensions_created" -eq 0 ] && [ ! -e "$EXTENSIONS_CONF.bak-pjsip" ]; then
                cp -a "$EXTENSIONS_CONF" "$EXTENSIONS_CONF.bak-pjsip"
            fi
            cat "$merged_extensions" > "$EXTENSIONS_CONF"
        fi
        rm -f "$context_patch" "$body_patch" "$merged_extensions"
    else
        if [ "$extensions_created" -eq 0 ] && [ ! -e "$EXTENSIONS_CONF.bak-pjsip" ]; then
            cp -a "$EXTENSIONS_CONF" "$EXTENSIONS_CONF.bak-pjsip"
        fi
        if [ -s "$EXTENSIONS_CONF" ]; then
            printf '\n' >> "$EXTENSIONS_CONF"
        fi
        cat "$context_patch" >> "$EXTENSIONS_CONF"
        rm -f "$context_patch"
    fi
done

PJSIP_CONF="$AST_DIR/pjsip.conf"
pjsip_created=0
if [ ! -e "$PJSIP_CONF" ]; then
    install -m 0640 /dev/null "$PJSIP_CONF"
    pjsip_created=1
fi

transport_merged=$(mktemp "${PJSIP_CONF}.amerimon.XXXXXX")
if awk '
    /^[[:space:]]*[;#]/ { next }
    /^[[:space:]]*\[/ {
        section = $0
        sub(/^[[:space:]]*\[/, "", section)
        sub(/\].*$/, "", section)
        sub(/[[:space:]]*\(!\)[[:space:]]*$/, "", section)
        if (tolower(section) == "transport-udp") found = 1
    }
    END { exit !found }
' "$PJSIP_CONF"; then
    rm -f "$transport_merged"
    echo "PJSIP: [transport-udp] ya existe; se conserva sin cambios."
else
    if ! awk '
        { print }
        END {
            if (NR > 0) print ""
            print "[transport-udp]"
            print "type=transport"
            print "protocol=udp"
            print "bind=0.0.0.0"
        }
    ' "$PJSIP_CONF" > "$transport_merged"; then
        rm -f "$transport_merged"
        echo "Error: no se pudo integrar el transporte UDP en $PJSIP_CONF." >&2
        exit 1
    fi
    if [ "$pjsip_created" -eq 0 ] && [ ! -e "$PJSIP_CONF.bak-pjsip" ]; then
        cp -a "$PJSIP_CONF" "$PJSIP_CONF.bak-pjsip"
    fi
    cat "$transport_merged" > "$PJSIP_CONF"
    rm -f "$transport_merged"
fi

module_running() {
    asterisk -rx "module show like $1" 2>/dev/null |
        awk -v module="$1" '
            $1 == module &&
            /[[:space:]]Running([[:space:]]|$)/ &&
            !/Not[[:space:]]+Running/ { found = 1 }
            END { exit !found }
        '
}

module_location() {
    if [ -n "$configured_module_dir" ] && [ -f "$configured_module_dir/$1" ]; then
        printf '%s\n' "$configured_module_dir/$1"
        return
    fi
    for module_dir in ${ASTERISK_MODULE_DIRS:-/usr/lib/*/asterisk/modules /usr/lib/asterisk/modules /usr/lib64/asterisk/modules /usr/local/lib/asterisk/modules}; do
        if [ -f "$module_dir/$1" ]; then
            printf '%s\n' "$module_dir/$1"
            return
        fi
    done
    printf 'no encontrado en astmoddir ni en las rutas de búsqueda\n'
}

if command -v asterisk >/dev/null 2>&1 && asterisk -rx 'core show version' >/dev/null 2>&1; then
    configured_module_dir=
    if [ -r "$AST_DIR/asterisk.conf" ]; then
        configured_module_dir=$(awk '
            { sub(/;.*/, "") }
            /^[[:space:]]*\[/ {
                in_directories = ($0 ~ /^[[:space:]]*\[directories\]([[:space:]]*\(!\))?[[:space:]]*$/)
                next
            }
            in_directories && /^[[:space:]]*astmoddir[[:space:]]*=>?/ {
                sub(/^[[:space:]]*astmoddir[[:space:]]*=>?[[:space:]]*/, "")
                sub(/[[:space:]]+$/, "")
                directory = $0
            }
            END { print directory }
        ' "$AST_DIR/asterisk.conf")
    fi
    # Load infrastructure before SIP resources and the channel driver.
    pending_modules=$(printf '%s\n' "$AVAILABLE_MODULES" | awk '
        {
            for (i = 1; i <= NF; i++) {
                module = $i
                priority = 4
                if (module ~ /^res_sorcery_/ || module == "res_pjproject.so" || module == "res_rtp_asterisk.so") priority = 1
                else if (module == "res_pjsip.so") priority = 2
                else if (module == "res_pjsip_session.so" || module == "res_pjsip_pubsub.so") priority = 3
                else if (module == "chan_pjsip.so") priority = 5
                print priority, i, module
            }
        }
    ' | sort -n -k1,1 -k2,2 | awk '{ print $3 }')
    while [ -n "$pending_modules" ]; do
        next_pending=
        progress=0
        : > "$load_errors"
        for module in $pending_modules; do
            if module_running "$module"; then
                continue
            fi
            if load_output=$(asterisk -rx "module load $module" 2>&1); then
                load_status=0
            else
                load_status=$?
            fi
            if module_running "$module"; then
                progress=1
            else
                next_pending="$next_pending $module"
                printf 'Error: Asterisk no confirmó la carga de %s en estado Running (código CLI %s).\nRespuesta de Asterisk: %s\n' \
                    "$module" "$load_status" "$load_output" >> "$load_errors"
            fi
        done
        if [ -n "$next_pending" ] && [ "$progress" -eq 0 ]; then
            required_failed=0
            for module in $next_pending; do
                location=$(module_location "$module")
                severity=Error
                if [ "$module" = res_pjsip_publish_asterisk.so ]; then
                    severity=Aviso
                    printf 'Aviso: módulo opcional %s no activo; solo afecta al intercambio de eventos PUBLISH entre servidores, no a las extensiones SIP.\n' "$module" >&2
                    echo "Revise el registro de Asterisk: este módulo puede rechazar la carga si falta Entity ID (consulte core show settings), aunque el archivo exista. No se modificó entityid." >&2
                else
                    required_failed=1
                fi
                printf 'Ubicación de %s: %s\n' "$module" "$location" >&2
                awk -v module="$module" -v severity="$severity" '
                    /^Error: Asterisk no confirmó la carga de / {
                        selected = index($0, "la carga de " module " ") > 0
                        if (selected) sub(/^Error:/, severity ":")
                    }
                    selected { print }
                ' "$load_errors" >&2
            done
            if [ "$required_failed" -eq 1 ]; then
                echo "Error: no se pudo activar PJSIP. La configuración y sus respaldos se conservaron; no se reinició Asterisk." >&2
                exit 1
            fi
            break
        fi
        pending_modules=$next_pending
    done
    echo "PJSIP: todos los módulos requeridos están en estado Running."

    if ! command -v netstat >/dev/null 2>&1; then
        echo "Error: no se encontró netstat; instala el paquete net-tools para verificar el transporte UDP de PJSIP." >&2
        exit 1
    fi

    transport_is_active() {
        transport_output=$(asterisk -rx 'pjsip show transport transport-udp' 2>&1) || return 1
        printf '%s\n' "$transport_output" | grep -qi 'transport-udp' &&
            printf '%s\n' "$transport_output" | grep -Eq '(^|[[:space:]])0\.0\.0\.0:5060([[:space:]]|$)' || return 1
        netstat_output=$(netstat -uln 2>&1) || return 1
        printf '%s\n' "$netstat_output" | awk '$1 ~ /^udp/ && $4 ~ /:5060$/ { found = 1 } END { exit !found }'
    }

    reload_output=
    reload_status=0
    if transport_is_active; then
        echo "PJSIP: [transport-udp] está activo en 0.0.0.0:5060/UDP."
    else
        echo "PJSIP: el transporte UDP no está activo; se intenta recargar res_pjsip.so."
        if reload_output=$(asterisk -rx 'module reload res_pjsip.so' 2>&1); then
            reload_status=0
        else
            reload_status=$?
        fi
        if transport_is_active; then
            echo "PJSIP: [transport-udp] está activo en 0.0.0.0:5060/UDP después de recargar res_pjsip.so."
        else
            echo "Aviso: Asterisk no confirmó [transport-udp] activo en 0.0.0.0:5060/UDP, incluso después de intentar module reload res_pjsip.so (código CLI $reload_status). La configuración se conservó; revisa 'pjsip show transport transport-udp' y la respuesta de Asterisk antes de reiniciar el servicio." >&2
            if [ -n "$reload_output" ]; then
                printf 'Respuesta de module reload: %s\n' "$reload_output" >&2
            fi
            if [ -n "${transport_output:-}" ]; then
                printf 'Respuesta de Asterisk: %s\n' "$transport_output" >&2
            fi
            if [ -n "${netstat_output:-}" ]; then
                printf 'Sockets UDP detectados: %s\n' "$netstat_output" >&2
            fi
        fi
    fi
else
    echo "Aviso: Asterisk no está activo; no se pudo comprobar si [transport-udp] escucha en 0.0.0.0:5060/UDP. La configuración se aplicará al iniciar Asterisk." >&2
fi
