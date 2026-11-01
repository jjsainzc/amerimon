#!/bin/sh
set -e

command -v ip >/dev/null 2>&1 || {
    echo "Error: falta ip; instala iproute2 para detectar las interfaces PJSIP." >&2
    exit 1
}
addresses=$(ip -o -4 addr show up) || {
    echo "Error: no se pudieron consultar las direcciones IPv4." >&2
    exit 1
}
interfaces=$(printf '%s\n' "$addresses" | awk '
    $3 == "inet" {
        split($4, cidr, "/")
        split(cidr[1], octet, ".")
        if (octet[1] == 127 || octet[1] == 169 && octet[2] == 254 || octet[1] >= 224) next
        interface = $2
        sub(/@.*/, "", interface)
        private = octet[1] == 10 || octet[1] == 172 && octet[2] >= 16 && octet[2] <= 31 ||
                  octet[1] == 192 && octet[2] == 168
        print interface, cidr[1], private ? "private" : "public"
    }
')

case "${1:-}" in
    bind)
        routes=$(ip -4 route show default) || {
            echo "Error: no se pudo consultar la ruta predeterminada." >&2
            exit 1
        }
        default_interface=$(printf '%s\n' "$routes" | awk '
            { for (i = 1; i < NF; i++) if ($i == "dev") { print $(i+1); exit } }
        ')
        private_ip=$(printf '%s\n' "$interfaces" | awk -v preferred="$default_interface" '
            $3 == "private" {
                if (first == "") first = $2
                if ($1 == preferred) { print $2; found = 1; exit }
            }
            END { if (!found && first != "") print first }
        ')
        if [ -n "$private_ip" ]; then
            printf '%s\n' "$private_ip"
        elif [ -n "$interfaces" ]; then
            printf '0.0.0.0\n'
        else
            echo "Error: no hay una IPv4 utilizable para el transporte PJSIP." >&2
            exit 1
        fi
        ;;
    protect)
        if command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
            backend=firewalld
        elif command -v iptables >/dev/null 2>&1; then
            backend=iptables
        elif command -v firewall-cmd >/dev/null 2>&1; then
            echo "PJSIP: se inicia el firewalld instalado para proteger UDP/5060." >&2
            systemctl enable --now firewalld
            firewall-cmd --state >/dev/null
            backend=firewalld
        else
            echo "PJSIP: no hay firewall disponible; se instala iptables para proteger UDP/5060." >&2
            command -v apt-get >/dev/null 2>&1 || {
                echo "Error: instala iptables o firewalld antes de habilitar PJSIP." >&2
                exit 1
            }
            apt-get install -y iptables
            command -v iptables >/dev/null 2>&1 || {
                echo "Error: iptables no está disponible después de instalarlo; PJSIP no se habilitará." >&2
                exit 1
            }
            backend=iptables
        fi
        # Raw PREROUTING precede ACCEPT; LOCAL cubre también nuevas IP de VPN/túneles,
        # pero no bloquea tráfico reenviado ni el resto de puertos.
        chain=AMERIMON-PJSIP
        add_rule() {
            if [ "$backend" = firewalld ]; then
                if ! firewall-cmd $scope --direct --query-rule ipv4 raw "$@" >/dev/null; then
                    firewall-cmd $scope --direct --add-rule ipv4 raw "$@"
                fi
                firewall-cmd $scope --direct --query-rule ipv4 raw "$@" >/dev/null
            else
                rule_chain=$1
                shift 2
                if ! iptables -w -t raw -C "$rule_chain" "$@" 2>/dev/null; then
                    if [ "$rule_chain" = PREROUTING ]; then
                        iptables -w -t raw -I "$rule_chain" 1 "$@"
                    else
                        iptables -w -t raw -A "$rule_chain" "$@"
                    fi
                fi
                iptables -w -t raw -C "$rule_chain" "$@"
            fi
        }
        for scope in runtime --permanent; do
            if [ "$backend" = firewalld ]; then
                [ "$scope" != runtime ] || scope=
                if ! firewall-cmd $scope --direct --query-chain ipv4 raw "$chain" >/dev/null; then
                    firewall-cmd $scope --direct --add-chain ipv4 raw "$chain"
                fi
            elif ! iptables -w -t raw -S "$chain" >/dev/null 2>&1; then
                iptables -w -t raw -N "$chain"
            fi
            for network in 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 127.0.0.0/8 169.254.0.0/16; do
                add_rule "$chain" 0 -d "$network" -j RETURN
            done
            add_rule "$chain" 10 -m addrtype --dst-type LOCAL -j DROP
            add_rule PREROUTING 0 -p udp --dport 5060 -j "$chain"
            [ "$backend" = firewalld ] || break
        done
        echo "PJSIP: $backend bloquea UDP/5060 entrante a IPv4 locales no privadas, incluidas VPN y túneles."
        ;;
    *)
        echo "Error: uso: pjsip-network.sh bind|protect" >&2
        exit 1
        ;;
esac
