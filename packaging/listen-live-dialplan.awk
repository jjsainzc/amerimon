# Solo migra bloques generados anteriormente; conserva extensiones personalizadas.
function flush_monitor(    compact, i, line) {
    if (!monitor_count) return
    compact = ""
    for (i = 1; i <= monitor_count; i++) {
        line = monitor_block[i]
        gsub(/[ \t\r]/, "", line)
        if (line != "" && line !~ /^;/) compact = compact line "\n"
    }
    if (compact != "[amerimon-listen-node]\nexten=>h,1,NoOp(ListenLivenodechannelended)\nsame=>n,Hangup()\nexten=>_X.,1,NoOp(ListenLivemonitornodo${EXTEN})\nsame=>n,Answer()\nsame=>n,Rpt(${EXTEN},Pm)\nsame=>n,Hangup()\n") {
        for (i = 1; i <= monitor_count; i++) print monitor_block[i]
    }
    monitor_count = 0
}
function flush_node(    compact, i) {
    if (!count) return
    compact = ""
    for (i = 1; i <= count; i++) {
        line = block[i]
        gsub(/[ \t\r]/, "", line)
        compact = compact line "\n"
    }
    if (compact ~ /^exten=>nodo,1,NoOp\(Espiandonodolocal\$\{NODE\}\)\nsame=>n,Answer\(\)\nsame=>n,(ChanSpy\((SimpleUSB\/\$\{NODE\}|Local\/pseudo@default),q\)|Rpt\(\$\{NODE\},Pm\))\nsame=>n,Hangup\(\)\n$/) {
        if (!replaced) {
            print "exten => nodo,1,Goto(amerimon-listen-node,${NODE},1)"
            replaced = 1
        }
    } else {
        for (i = 1; i <= count; i++) print block[i]
        if (compact ~ /^exten=>nodo,/ && compact != "exten=>nodo,1,Goto(amerimon-listen-node,${NODE},1)\n") {
            print "Aviso: bloque nodo personalizado conservado en listen-live-spy." > "/dev/stderr"
        }
    }
    count = 0
}
/^[ \t]*\[/ {
    flush_node()
    flush_monitor()
    monitor = ($0 ~ /^[ \t]*\[amerimon-listen-node\][ \t]*(;.*)?$/)
    spy = ($0 ~ /^[ \t]*\[listen-live-spy\][ \t]*(;.*)?$/)
}
monitor {
    monitor_block[++monitor_count] = $0
    next
}
spy && /^[ \t]*exten[ \t]*=>[ \t]*nodo[ \t]*,/ {
    flush_node()
    block[++count] = $0
    next
}
count {
    if ($0 ~ /^[ \t]*same[ \t]*=>/) {
        block[++count] = $0
        next
    }
    flush_node()
}
{ print }
END {
    flush_node()
    flush_monitor()
}
