# Patch only the standard peerip checks, preserving the access policy.
{
    if ($0 ~ /^[[:space:]]*\[/) {
        in_radio = ($0 ~ /^[[:space:]]*\[radio-secure\][[:space:]]*(;.*)?$/)
        if (in_radio) {
            found++
        }
    }
    if (in_radio && $0 ~ /^[[:space:]]*same[[:space:]]*=>[[:space:]]*n,NoOp\(The Channel IP is \$\{CHANNEL\(peerip\)\}\)[[:space:]]*$/) {
        checks++
        print "    same => n,Set(CHAN_IP=)"
        print "    same => n,GotoIf($[\"${CHANNEL(channeltype)}\" = \"IAX2\" | \"${CHANNEL(channeltype)}\" = \"SIP\"]?read_peerip:check_ip)"
        print "    same => n(read_peerip),Set(CHAN_IP=${CHANNEL(peerip)})"
        print "    same => n(check_ip),NoOp(The Channel IP is ${CHAN_IP})"
        next
    }
    if (in_radio && $0 ~ /^[[:space:]]*same[[:space:]]*=>[[:space:]]*n\(continue\),GotoIf/) {
        replacements += sub(/\$\{CHANNEL\(peerip\)\}/, "${CHAN_IP}")
    }
    print
}
END {
    if (!found) {
        exit 3
    }
    if (found > 1 || checks != replacements || checks > 1) {
        exit 2
    }
}
