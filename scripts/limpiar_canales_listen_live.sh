#!/bin/bash
# Uso: sudo limpiar_canales_listen_live.sh [--check|--clean]
#   --check: muestra canales y procesos residuales (modo predeterminado).
#   --clean: solicita hangup y termina procesos EAGI; pide confirmar con LIMPIAR.
# Aviso: --clean interrumpe todas las escuchas Listen Live activas.

set -u

mode="${1:---check}"
if [[ $# -gt 1 ]] || [[ "$mode" != "--check" && "$mode" != "--clean" ]]; then
	echo "Uso: $0 [--check|--clean]"
	exit 2
fi
if [[ $EUID -ne 0 ]]; then
	echo "Ejecuta como root: sudo $0 $mode"
	exit 1
fi

channel_output=$(asterisk -rx "core show channels concise") || exit 1
get_channels() {
	printf '%s\n' "$1" | awk -F'!' '$1 ~ /^Local\/.*@listen-live-spy-/ { print $1 }' | sort -u
}
channels=$(get_channels "$channel_output")

refresh_processes() {
	mapfile -t script_pids < <(pgrep -f '^/bin/bash /usr/local/bin/audio_eagi\.sh ' || true)
	mapfile -t nc_pids < <(pgrep -f '^nc 127\.0\.0\.1 8086$' || true)
	cat_pids=()
	for script_pid in "${script_pids[@]}"; do
		while IFS= read -r child_pid; do
			[[ -z "$child_pid" ]] && continue
			child_args=$(ps -o args= -p "$child_pid" 2>/dev/null || true)
			if [[ "$child_args" == "cat" || "$child_args" == */cat ]]; then
				cat_pids+=("$child_pid")
			fi
		done < <(pgrep -P "$script_pid" || true)
	done
}
refresh_processes

show_report() {
	local output="$1"
	local current_channels
	local -a process_pids=("${script_pids[@]}" "${cat_pids[@]}" "${nc_pids[@]}")
	current_channels=$(get_channels "$output")
	if [[ -n "$current_channels" ]]; then
		echo "Canales Local Listen Live:"
		printf '%s\n' "$current_channels"
	else
		echo "Sin canales Local Listen Live."
	fi

	if ((${#process_pids[@]} > 0)); then
		echo "Procesos EAGI activos:"
		ps -o pid,ppid,stat,etime,args -p "$(IFS=,; echo "${process_pids[*]}")" 2>/dev/null || true
	else
		echo "Sin procesos audio_eagi.sh, cat o nc en el listener."
	fi

	local zombies
	zombies=$(ps -eo pid=,ppid=,stat=,etime=,args= | awk '$3 ~ /^Z/ && $5 == "[audio_eagi.sh]"')
	if [[ -n "$zombies" ]]; then
		echo "Procesos zombie (no se pueden matar directamente; los debe recoger Asterisk):"
		printf '%s\n' "$zombies"
	fi
}

show_report "$channel_output"
if [[ "$mode" == "--check" ]]; then
	echo "Solo consulta. Para solicitar limpieza: sudo $0 --clean"
	exit 0
fi

echo "--clean interrumpira todas las escuchas Listen Live actuales."
read -r -p "Escribe LIMPIAR para continuar: " confirmation
if [[ "$confirmation" != "LIMPIAR" ]]; then
	echo "Cancelado."
	exit 1
fi

while IFS= read -r channel; do
	[[ -z "$channel" ]] && continue
	asterisk -rx "channel request hangup $channel"
done <<< "$channels"

# No matar el grupo de procesos: comparte PGID con Asterisk. Terminar solo los PID de EAGI.
for pid in "${cat_pids[@]}" "${nc_pids[@]}" "${script_pids[@]}"; do
	[[ -z "$pid" ]] || kill -TERM "$pid" 2>/dev/null || true
done

refresh_processes
channel_output=$(asterisk -rx "core show channels concise") || exit 1
echo "Estado despues de solicitar la limpieza:"
show_report "$channel_output"
