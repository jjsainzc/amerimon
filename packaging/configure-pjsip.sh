#!/bin/sh
set -e

AST_DIR=${AST_DIR:-/etc/asterisk}
PJSIP_MODULES_PATCH=${PJSIP_MODULES_PATCH:-/usr/share/amerimon/pjsip-modules-patch.conf}
PJSIP_EXTENSIONS_PATCH=${PJSIP_EXTENSIONS_PATCH:-/usr/share/amerimon/pjsip-extensions-sip-phones-patch.conf}
PJSIP_MODULES_MERGER=${PJSIP_MODULES_MERGER:-/usr/lib/amerimon/merge-pjsip-modules.awk}
ASTERISK_MODULE_DIRS=${ASTERISK_MODULE_DIRS:-"/usr/lib/*/asterisk/modules /usr/lib/asterisk/modules"}

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

available_patch=$(mktemp "${MODULES_CONF}.pjsip-patch.XXXXXX")
merged_modules=$(mktemp "${MODULES_CONF}.pjsip-merged.XXXXXX")
trap 'rm -f "$available_patch" "$merged_modules"' EXIT HUP INT TERM

while IFS= read -r line || [ -n "$line" ]; do
    module=$(printf '%s\n' "$line" | sed -nE 's/^[[:space:]]*load[[:space:]]*(=>|=)[[:space:]]*([^[:space:];]+).*/\2/p')
    if [ -z "$module" ]; then
        case "$line" in
            *noload*)
                printf '%s\n' "$line" >> "$available_patch"
                ;;
        esac
        continue
    fi

    module_found=0
    for module_file in $ASTERISK_MODULE_DIRS/"$module"; do
        if [ -f "$module_file" ]; then
            module_found=1
            break
        fi
    done
    if [ "$module_found" -eq 1 ]; then
        printf '%s\n' "$line" >> "$available_patch"
    else
        echo "Aviso: módulo PJSIP no instalado: $module; se conserva modules.conf sin esa directiva." >&2
    fi
done < "$PJSIP_MODULES_PATCH"

if awk -v config="$MODULES_CONF" -v patch="$available_patch" -f "$PJSIP_MODULES_MERGER" "$MODULES_CONF" > "$merged_modules"; then
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
