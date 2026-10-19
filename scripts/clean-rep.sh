#!/bin/bash

# El Makefile exporta REPO_DIR; sin él se usa la ruta por defecto.
REPO_DIR="${REPO_DIR:-/var/www/html/repo}"
KEEP_COUNT=3
DRY_RUN=false

cd "$REPO_DIR" || exit 1

[ "$DRY_RUN" = true ] && echo "=== MODO DE PRUEBA (DRY RUN) ACTIVADO ==="

# 1. Limpiar amerimon por arquitectura y 2. Limpiar amerinet-web en un solo bloque
for pattern in  "serverutil_*_linux_amd64.deb" "serverutil_*_linux_arm64.deb"  "amerimon_*_linux_amd64.deb" "amerimon_*_linux_arm64.deb" "amerimon_*_linux_armhf.deb" "amerinet-web_*_all.deb"; do
  # sort -V ordena de menor a mayor versión, tail -n +$((KEEP_COUNT + 1)) deja solo las que exceden el límite a borrar
  for file in $(ls -1 $pattern 2>/dev/null | sort -V | head -n -"$KEEP_COUNT"); do
    [ -e "$file" ] || continue
    if [ "$DRY_RUN" = true ]; then
      echo "[DRY-RUN] Borraría: $file"
    else
      echo "Borrando: $file"
      rm -f "$file"
    fi
  done
done

# Regenerar repositorio
if [ "$DRY_RUN" = false ]; then
  echo "=== Regenerando Packages.gz ==="
  ./update-repo.sh
fi

echo "Proceso completado."
