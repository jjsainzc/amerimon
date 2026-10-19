#!/bin/sh
set -e

# Skip on upgrade; only stop when the package is really being removed.
if [ "$1" = "remove" ] && [ -d /run/systemd/system ]; then
    systemctl disable --now amerimon-astdb.timer || true
    systemctl disable --now amerimon.service || true
fi
