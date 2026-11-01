BEGIN {
    while ((getline line < patch) > 0) {
        if (line ~ /^[[:space:]]*\[allstar-public\][[:space:]]*(;.*)?$/) {
            in_patch = 1
            continue
        }
        if (in_patch) {
            if (line ~ /^[[:space:]]*\[/) {
                break
            }
            patch_lines[++patch_count] = line
        }
    }
    close(patch)
    if (!in_patch || patch_count == 0) {
        exit 2
    }
}

{
    if ($0 ~ /^[[:space:]]*\[allstar-public\][[:space:]]*(;.*)?$/) {
        print
        found = 1
        replacing = 1
        for (i = 1; i <= patch_count; i++) {
            print patch_lines[i]
        }
        next
    }
    if (replacing && $0 ~ /^[[:space:]]*\[/) {
        replacing = 0
    }
    if (!replacing) {
        print
    }
}

END {
    if (!found) {
        exit 3
    }
}
