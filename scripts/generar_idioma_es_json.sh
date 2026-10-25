#!/bin/bash

# Rutas ajustadas según tu estructura actual
HTML_DIR="src/app"
OUTPUT_JSON="public/assets/i18n/es_auto.json"

# Asegurarnos de que el directorio de destino exista
mkdir -p "$(dirname "$OUTPUT_JSON")"

echo "{" > "$OUTPUT_JSON"

first=1

# Buscamos texto visible dentro de etiquetas HTML en la carpeta src/app
grep -rohE '>[A-Za-zÁÉÍÓÚáéíóúÑñ0-9/ _–—\.,®©]+<0' "$HTML_DIR" 2>/dev/null | sed 's/>//g; s/<//g; s/^[[:space:]]*//; s/[[:space:]]*$//' | sort -u | while read -r texto; do
    if [ -n "$texto" ]; then
        # Creamos una clave basada en el texto (mayúsculas y guiones bajos)
        clave=$(echo "$texto" | tr '[:lower:]' '[:upper:]' | iconv -f utf8 -t ascii//TRANSLIT | sed 's/[^A-Z0-9]/_/g' | sed 's/__*/_/g' | sed 's/^_//;s/_$//')

        if [ ${#clave} -gt 2 ]; then
            if [ $first -eq 0 ]; then
                echo "," >> "$OUTPUT_JSON"
            fi
            echo -e "  \"$clave\": \"$texto\"" >> "$OUTPUT_JSON"
            first=0
        fi
    fi
done

echo "" >> "$OUTPUT_JSON"
echo "}" >> "$OUTPUT_JSON"

echo "¡Archivo JSON generado con éxito en $OUTPUT_JSON!"
