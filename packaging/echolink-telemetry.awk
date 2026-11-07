function trim(value) {
    gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
    return value
}

function fail(message) {
    print "EchoLink telemetry: " message > "/dev/stderr"
    failed = 1
    exit 1
}

{
    clean = $0
    sub(/[;#].*$/, "", clean)
    clean = trim(clean)
    if (clean ~ /^\[/) {
        section = clean
        sub(/^\[[[:space:]]*/, "", section)
        sub(/[[:space:]]*\].*$/, "", section)
        section = tolower(section)
    }
}

FILENAME == ARGV[1] {
    if (section ~ /^el[0-9]+$/ && tolower(clean) ~ /^astnode[[:space:]]*=/) {
        node = clean
        sub(/^[^=]*=/, "", node)
        node = trim(node)
        if (node !~ /^[0-9]+$/) fail("invalid astnode in [" section "]")
        nodes[node] = 1
    }
    next
}

FILENAME == ARGV[2] {
    lines[++count] = $0
    sections[count] = section
    if (clean ~ /^\[/) {
        if (section == "node-main") {
            if (++templateHeaders > 1 || clean !~ /\([[:space:]]*![[:space:]]*\)/)
                fail("duplicate or invalid RPT template [node-main]")
            template = 1
        }
        if (section in nodes) {
            if (++headers[section] > 1 || clean ~ /\([[:space:]]*![[:space:]]*\)/)
                fail("duplicate or template RPT section [" section "]")
        }
    }
}

END {
    if (failed) exit 1
    found = template
    for (node in nodes) {
        if (headers[node] != 1) {
            if (!template)
                print "EchoLink telemetry: missing RPT section [" node "], skipped" > "/dev/stderr"
            delete nodes[node]
            continue
        }
        found = 1
    }
    if (!found) {
        print "EchoLink telemetry: no [node-main] template or astnode with matching RPT section found" > "/dev/stderr"
        exit 3
    }
    for (i = 1; i <= count; i++) {
        clean = trim(lines[i])
        target = sections[i] in nodes || (template && sections[i] == "node-main")
        if (clean ~ /^\[/ && target) {
            print lines[i]
            print "echolinkdefault = 1"
        } else if (!(target && tolower(clean) ~ /^echolinkdefault[[:space:]]*=/)) {
            print lines[i]
        }
    }
}
