#!/bin/bash

HTML_DIR="src/app"
OUTPUT_JSON="public/assets/i18n/es_auto.json"

# Asegurar que el directorio de salida exista
mkdir -p "$(dirname "$OUTPUT_JSON")"

echo "{" > "$OUTPUT_JSON"

# Usamos grep para extraer todo el texto que está entre etiquetas HTML (>texto<)
# Luego limpiamos espacios, etiquetas vacías y variables de interpolación de Angular {{ }}
grep -rohE '>[^<]+<' "$HTML_DIR" 2>/dev/null | \
  sed 's/>//g; s/<//g; s/^[[:space:]]*//; s/[[:space:]]*$//' | \
  grep -v '^{' | grep -v '^}$' | grep -v '^{{' | \
  awk 'NF' | sort -u | while IFS= read -r texto; do
    # Omitir textos muy cortos o que sean solo símbolos
    if [ ${#texto} -gt 1 ]; then
        # Generar clave limpia en mayúsculas (ej: "Personalizar cabecera" -> "PERSONALIZAR_CABECERA")
        clave=$(echo "$texto" | iconv -f utf8 -t ascii//TRANSLIT 2>/dev/null | tr '[:lower:]' '[:upper:]' | sed 's/[^A-Z0-9]/_/g' | sed 's/__*/_/g' | sed 's/^_//;s/_$//')

        if [ -n "$clave" ]; then
            # Acumular para escribir en el JSON
            echo "  \"$clave\": \"$texto\"," >> "$OUTPUT_JSON"
        fi
    fi
done

# Quitar la última coma sobrante del archivo JSON y cerrar la llave
sed -i '$ s/,$//' "$OUTPUT_JSON"
echo "}" >> "$OUTPUT_JSON"

echo "¡Listo! Archivo generado en $OUTPUT_JSON"
