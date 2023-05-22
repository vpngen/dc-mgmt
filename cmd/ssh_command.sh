#!/bin/sh

# interpret first argument as command
# pass rest args to scripts


if [ $# -eq 0 ]; then 
    printdef
fi

cmd=${1}; shift
basedir=$(dirname "$0")

printdef() {
    echo "Usage: <command> <args...>"
    exit 1
}

kdsync() {
        # shellcheck source=/dev/null
        . /etc/vg-dc-vpnapi/kdsync.env

        export KDSYNC_SERVER_ADDR
        export KDSYNC_SERVER_PORT

        # shellcheck source=/dev/null
        . /etc/vg-dc-mgmt/dc-name.env

        /usr/bin/flock -x -E 0 -n /tmp/kdsync.lock "${basedir}"/kdsync.sh 2>&1 | /usr/bin/logger -p local0.notice -t KDSYNC
}

if [ "addbrigade" = "${cmd}" ]; then
        "${basedir}"/addbrigade "$@"
        kdsync
elif [ "delbrigade" = "${cmd}" ]; then
        "${basedir}"/delbrigade "$@"
        kdsync
elif [ "replacebrigadier" = "${cmd}" ]; then
    "${basedir}"/replacebrigadier "$@"
elif [ "getwasted" = "${cmd}" ]; then
    "${basedir}"/getwasted "$@"
elif [ "checkbrigade" = "${cmd}" ]; then
    "${basedir}"/checkbrigade "$@"
elif [ "get_free_slots" = "${cmd}" ]; then
    "${basedir}"/get_free_slots "$@"
else
    echo "Unknown command: ${cmd}"
    printdef
fi
