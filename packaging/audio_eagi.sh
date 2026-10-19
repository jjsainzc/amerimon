#!/bin/bash
# /usr/local/bin/audio_eagi.sh
UUID=$1
PORT=${2:-8086}

# Consumir el encabezado AGI (termina con línea vacía)
while IFS= read -r line; do
    line=${line%$'\r'}
    [ -z "$line" ] && break
done

# Un FIFO permite terminar el lector de FD 3 cuando el servidor cierra TCP.
fifo_dir=$(mktemp -d "${TMPDIR:-/tmp}/audio-eagi.XXXXXX") || exit 1
fifo="$fifo_dir/stream"
if ! mkfifo "$fifo"; then
    rmdir "$fifo_dir"
    exit 1
fi

nc_pid=
producer_pid=
cleanup() {
    for pid in "$nc_pid" "$producer_pid"; do
        if [ -n "$pid" ]; then
            kill "$pid" 2>/dev/null || true
            wait "$pid" 2>/dev/null || true
        fi
    done
    rm -rf "$fifo_dir"
}
trap cleanup EXIT
trap 'exit 0' HUP INT TERM

# nc recibe el UUID primero y luego el audio; al salir nc, cleanup termina cat.
nc 127.0.0.1 "$PORT" < "$fifo" &
nc_pid=$!
(
    printf '%s' "$UUID"
    exec cat <&3
) > "$fifo" &
producer_pid=$!

wait "$nc_pid"
exit $?