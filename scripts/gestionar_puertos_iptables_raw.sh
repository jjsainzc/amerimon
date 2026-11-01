#!/bin/bash

# Asegurarse de ejecutar como root
if [ "$EUID" -ne 0 ]; then
  echo "[-] Por favor, ejecuta este script con sudo o como root."
  exit 1
fi

CHAIN_NAME="AMERIMON-PJSIP"

# Función para asegurar que la estructura base de la cadena exista
inicializar_cadena() {
    # Si la cadena no existe, la creamos y enlazamos a PREROUTING para el puerto por defecto (5060)
    if ! iptables -t raw -L "$CHAIN_NAME" -n >/dev/null 2>&1; then
        echo "[+] Creando cadena personalizada $CHAIN_NAME..."
        iptables -t raw -N "$CHAIN_NAME"

        # Agregar reglas base para permitir redes privadas
        iptables -t raw -A "$CHAIN_NAME" -s 0.0.0.0/0 -d 10.0.0.0/8 -j RETURN
        iptables -t raw -A "$CHAIN_NAME" -s 0.0.0.0/0 -d 172.16.0.0/12 -j RETURN
        iptables -t raw -A "$CHAIN_NAME" -s 0.0.0.0/0 -d 192.168.0.0/16 -j RETURN
        iptables -t raw -A "$CHAIN_NAME" -s 0.0.0.0/0 -d 127.0.0.0/8 -j RETURN
        iptables -t raw -A "$CHAIN_NAME" -s 0.0.0.0/0 -d 169.254.0.0/16 -j RETURN

        # Regla de bloqueo final para tráfico externo dirigido a IPs locales
        iptables -t raw -A "$CHAIN_NAME" -m addrtype --dst-type LOCAL -j DROP
    fi
}

mostrar_estado() {
    echo ""
    echo "=================================================="
    echo " ESTADO ACTUAL DE LA TABLA RAW ($CHAIN_NAME)"
    echo "=================================================="
    iptables -t raw -L PREROUTING -v -n --line-numbers
    echo "--------------------------------------------------"
    iptables -t raw -L "$CHAIN_NAME" -v -n --line-numbers
    echo "=================================================="
    echo ""
}

# Menú de opciones
case "$1" in
    add)
        if [ -z "$2" ]; then
            echo "[-] Error: Debes especificar un puerto o rango. Ej: ./gestionar_puertos_iptables_raw.sh add 5061"
            exit 1
        fi
        inicializar_cadena
        # Verificar si ya existe la regla para ese puerto en PREROUTING
        if iptables -t raw -C PREROUTING -p udp --dport "$2" -j "$CHAIN_NAME" 2>/dev/null; then
            echo "[!] El puerto $2 ya está protegido en la tabla raw."
        else
            iptables -t raw -A PREROUTING -p udp --dport "$2" -j "$CHAIN_NAME"
            echo "[+] Puerto $2 añadido y protegido exitosamente."
        fi
        mostrar_estado
        ;;
    remove|del)
        if [ -z "$2" ]; then
            echo "[-] Error: Debes especificar un puerto a quitar. Ej: ./gestionar_puertos_iptables_raw.sh remove 5061"
            exit 1
        fi
        if iptables -t raw -C PREROUTING -p udp --dport "$2" -j "$CHAIN_NAME" 2>/dev/null; then
            iptables -t raw -D PREROUTING -p udp --dport "$2" -j "$CHAIN_NAME"
            echo "[+] Puerto $2 removido de la protección raw."
        else
            echo "[-] El puerto $2 no se encontró enlazado en la tabla raw."
        fi
        mostrar_estado
        ;;
    list|status)
        mostrar_estado
        ;;
    *)
        echo "Uso del script:"
        echo "  sudo ./gestionar_puertos_iptables_raw.sh add <puerto>     (Ej: sudo ./gestionar_puertos_iptables_raw.sh add 5060)"
        echo "  sudo ./gestionar_puertos_iptables_raw.sh remove <puerto>  (Ej: sudo ./gestionar_puertos_iptables_raw.sh remove 5060)"
        echo "  sudo ./gestionar_puertos_iptables_raw.sh list           (Ver estado actual)"
        exit 1
        ;;
esac
