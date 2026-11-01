#!/bin/bash

# Asegurarse de ejecutar como root
if [ "$EUID" -ne 0 ]; then
  echo "[-] Por favor, ejecuta este script con sudo o como root."
  exit 1
fi

# Definir la zona de firewalld a utilizar (por defecto public)
ZONE="public"

mostrar_estado() {
    echo ""
    echo "=================================================="
    echo " REGLAS DIRECTAS DE FIREWALLD PARA 5060 / UDP"
    echo "=================================================="
    firewall-cmd --direct --get-all-rules | grep -E "5060|AMERIMON"
    echo "=================================================="
    echo ""
}

# Menú de opciones
case "$1" in
    add)
        PORT="${2:-5060}"
        echo "[+] Añadiendo regla directa en firewalld para el puerto UDP $PORT..."

        # Firewalld permite inyectar reglas directamente en la tabla raw del kernel (PREROUTING)
        # 1. Permitir redes privadas (RETORNAR / aceptar sin procesar más en la cadena de bloqueo)
        firewall-cmd --permanent --direct --add-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 10.0.0.0/8 -j RETURN
        firewall-cmd --permanent --direct --add-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 172.16.0.0/12 -j RETURN
        firewall-cmd --permanent --direct --add-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 192.168.0.0/16 -j RETURN
        firewall-cmd --permanent --direct --add-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 127.0.0.0/8 -j RETURN
        firewall-cmd --permanent --direct --add-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 169.254.0.0/16 -j RETURN

        # 2. Bloquear (DROP) el tráfico externo dirigido a IPs locales del servidor en ese puerto
        firewall-cmd --permanent --direct --add-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -m addrtype --dst-type LOCAL -j DROP

        # Recargar para aplicar los cambios de forma persistente
        firewall-cmd --reload
        echo "[+] ¡Puerto $PORT protegido con éxito en firewalld!"
        mostrar_estado
        ;;

    remove|del)
        PORT="${2:-5060}"
        echo "[+] Removiendo reglas directas de firewalld para el puerto UDP $PORT..."

        # Eliminar las reglas asociadas a ese puerto de la tabla raw
        firewall-cmd --permanent --direct --remove-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 10.0.0.0/8 -j RETURN 2>/dev/null
        firewall-cmd --permanent --direct --remove-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 172.16.0.0/12 -j RETURN 2>/dev/null
        firewall-cmd --permanent --direct --remove-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 192.168.0.0/16 -j RETURN 2>/dev/null
        firewall-cmd --permanent --direct --remove-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 127.0.0.0/8 -j RETURN 2>/dev/null
        firewall-cmd --permanent --direct --remove-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -d 169.254.0.0/16 -j RETURN 2>/dev/null
        firewall-cmd --permanent --direct --remove-rule ipv4 raw PREROUTING 0 -p udp --dport "$PORT" -m addrtype --dst-type LOCAL -j DROP 2>/dev/null

        firewall-cmd --reload
        echo "[+] Puerto $PORT desprotegido."
        mostrar_estado
        ;;

    list|status)
        mostrar_estado
        ;;

    *)
        echo "Uso del script para Firewalld:"
        echo "  sudo ./gestionar_puertos_firewalld.sh add <puerto>     (Ej: sudo ./gestionar_puertos_firewalld.sh add 5060)"
        echo "  sudo ./gestionar_puertos_firewalld.sh remove <puerto>  (Ej: sudo ./gestionar_puertos_firewalld.sh remove 5060)"
        echo "  sudo ./gestionar_puertos_firewalld.sh list           (Ver reglas actuales)"
        exit 1
        ;;
esac
