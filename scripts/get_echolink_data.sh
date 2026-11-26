#!/usr/bin/env bash
set -euo pipefail

URL_LOGINS="https://www.echolink.org/logins.jsp"
ARCHIVO_SALIDA="echolink_nodes.json"
TEMP_DIR=$(mktemp -d)

trap 'rm -rf "$TEMP_DIR"' EXIT

echo "1. Descargando tabla en tiempo real desde EchoLink..."
curl -sSL \
  -A "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36" \
  "$URL_LOGINS" -o "$TEMP_DIR/logins.html"

if ! grep -qi "Current Logins" "$TEMP_DIR/logins.html"; then
    echo "Error: No se pudo obtener la tabla de logins.jsp." >&2
    exit 1
fi

echo "2. Convirtiendo tabla HTML a JSON..."

awk '
BEGIN {
    # Unir todo el HTML por filas de tabla <tr>
    RS = "</tr>|</TR>"
    print "["
    first = 1
}
/<tr|<TR/ {
    # Ignorar filas de encabezado
    if ($0 ~ /<th|<TH/) next

    # Extraer contenido de cada celda <td>...</td>
    linea = $0
    gsub(/\r|\n/, " ", linea)

    c1 = ""; c2 = ""; c3 = ""; c4 = ""; c5 = ""
    col = 0

    while (match(linea, /<[tT][dD][^>]*>.*<\/[tT][dD]>/)) {
        # Buscar el cierre de la primera celda
        inicio = index(linea, "<td")
        if (inicio == 0) inicio = index(linea, "<TD")
        resto = substr(linea, inicio)
        fin_tag = index(resto, ">")
        contenido = substr(resto, fin_tag + 1)

        cierre = index(contenido, "</td>")
        if (cierre == 0) cierre = index(contenido, "</TD>")
        if (cierre == 0) break

        celda = substr(contenido, 1, cierre - 1)
        linea = substr(contenido, cierre + 5)

        # Limpiar etiquetas HTML internas y entidades
        gsub(/<[^>]+>/, "", celda)
        gsub(/&nbsp;/, " ", celda)
        gsub(/&amp;/, "\\&", celda)
        gsub(/&quot;/, "\"", celda)
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", celda)

        col++
        if (col == 1) c1 = celda
        else if (col == 2) c2 = celda
        else if (col == 3) c3 = celda
        else if (col == 4) c4 = celda
        else if (col == 5) c5 = celda
    }

    # Verificar que la columna 5 sea un número de nodo válido
    if (col >= 5 && c5 ~ /^[0-9]+$/) {
        # Determinar tipo de estación según sufijo
        tipo = "USER"
        if (c1 ~ /-R$/) tipo = "REPEATER"
        else if (c1 ~ /-L$/) tipo = "LINK"
        else if (c1 ~ /^\*.*\*$/) tipo = "CONFERENCE"

        # Escapar caracteres especiales para JSON
        gsub(/\\/, "\\\\", c1); gsub(/"/, "\\\"", c1)
        gsub(/\\/, "\\\\", c2); gsub(/"/, "\\\"", c2)
        gsub(/\\/, "\\\\", c3); gsub(/"/, "\\\"", c3)
        gsub(/\\/, "\\\\", c4); gsub(/"/, "\\\"", c4)

        if (!first) printf ",\n"
        first = 0
        printf "  {\n    \"nodo\": %s,\n    \"indicativo\": \"%s\",\n    \"tipo\": \"%s\",\n    \"ubicacion\": \"%s\",\n    \"estado\": \"%s\",\n    \"hora_utc\": \"%s\"\n  }", c5, c1, tipo, c2, c3, c4
    }
}
END {
    print "\n]"
}
' "$TEMP_DIR/logins.html" > "$ARCHIVO_SALIDA"

TOTAL=$(grep -c '"indicativo":' "$ARCHIVO_SALIDA" || true)
echo "¡Listo! Se guardaron $TOTAL nodos activos en '$ARCHIVO_SALIDA'."

