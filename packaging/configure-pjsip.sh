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
trap 'rm -f "$merged_modules"' EXIT HUP INT TERM
AVAILABLE_MODULES=

while IFS= read -r line || [ -n "$line" ]; do
    module=$(printf '%s\n' "$line" | sed -nE 's/^[[:space:]]*load[[:space:]]*(=>|=)[[:space:]]*([^[:space:];]+).*/\2/p')
    if [ -n "$module" ]; then
        AVAILABLE_MODULES="$AVAILABLE_MODULES $module"
    fi
done < "$PJSIP_MODULES_PATCH"

if awk -v config="$MODULES_CONF" -v patch="$PJSIP_MODULES_PATCH" -f "$PJSIP_MODULES_MERGER" "$MODULES_CONF" > "$merged_modules"; then
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

if command -v asterisk >/dev/null 2>&1 && asterisk -rx 'core show version' >/dev/null 2>&1; then
    # Retry after dependencies have had a chance to load.
    for pass in 1 2; do
        for module in $AVAILABLE_MODULES; do
            if asterisk -rx "module show like $module" 2>/dev/null |
                awk -v module="$module" '$1 == module { found = 1 } END { exit !found }'; then
                continue
            fi
            load_output=$(asterisk -rx "module load $module" 2>&1) || {
                if [ "$pass" -eq 2 ]; then
                    echo "Aviso: falló la carga PJSIP de $module: $load_output" >&2
                fi
            }
            if [ "$pass" -eq 2 ] && ! asterisk -rx "module show like $module" 2>/dev/null |
                awk -v module="$module" '$1 == module { found = 1 } END { exit !found }'; then
                echo "Aviso: Asterisk no confirmó la carga de $module. Revisa sus dependencias y el registro de Asterisk." >&2
            fi
        done
    done
fi
