function module_name(line, fields, count) {
    sub(/;.*/, "", line)
    sub(/^[[:space:]]+/, "", line)
    sub(/[[:space:]]+$/, "", line)
    count = split(line, fields, /[[:space:]]+/)
    if (count >= 3 && fields[1] ~ /^(load|noload)$/ && fields[2] ~ /^(=>|=)$/) {
        return fields[3]
    }
    return ""
}

BEGIN {
    while ((getline line < config) > 0) {
        config_lines[++config_count] = line
        name = module_name(line)
        if (name != "") {
            configured[name] = 1
        }
    }
    close(config)

    while ((getline line < patch) > 0) {
        name = module_name(line)
        if (name != "" && !configured[name]) {
            configured[name] = 1
            additions[++addition_count] = line
        }
    }
    close(patch)
}

function add_missing_modules() {
    if (additions_added) {
        return
    }
    for (i = 1; i <= addition_count; i++) {
        print additions[i]
    }
    additions_added = 1
}

{
    if ($0 ~ /^[[:space:]]*\[modules\][[:space:]]*(;.*)?$/) {
        found_modules = 1
        in_modules = 1
    } else if ($0 ~ /^[[:space:]]*\[/ && in_modules) {
        add_missing_modules()
        in_modules = 0
    }
    print
}

END {
    if (in_modules) {
        add_missing_modules()
    } else if (!found_modules && addition_count > 0) {
        print ""
        print "[modules]"
        add_missing_modules()
    }
}
