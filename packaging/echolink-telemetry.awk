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
    if (clean ~ /^\[/ && section in nodes) {
        if (++headers[section] > 1 || clean ~ /\(!\)/)
            fail("duplicate or template RPT section [" section "]")
    }
}

END {
    if (failed) exit 1
    found = 0
    for (node in nodes) {
        found = 1
        if (headers[node] != 1) fail("missing RPT section [" node "]")
    }
    if (!found) {
        print "EchoLink telemetry: no astnode found" > "/dev/stderr"
        exit 3
    }
    for (i = 1; i <= count; i++) {
        clean = trim(lines[i])
        if (clean ~ /^\[/ && sections[i] in nodes) {
            print lines[i]
            print "echolinkdefault = 1"
        } else if (!(sections[i] in nodes && tolower(clean) ~ /^echolinkdefault[[:space:]]*=/)) {
            print lines[i]
        }
    }
}
