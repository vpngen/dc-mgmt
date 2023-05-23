#!/bin/sh

# interpret first argument as command
# pass rest args to scripts

printdef() {
    echo "Usage: <command> <args...>"
    exit 1
}

if [ $# -eq 0 ]; then 
    printdef
fi

cmd=${1}; shift
basedir=$(dirname "$0")

vpn_works_keysesks_sync() {
        # shellcheck source=/dev/null
        . /etc/vg-dc-vpnapi/vpn-works-keydesks-sync.env

        export VPN_WORKS_KEYDESKS_SERVER_ADDR
        export VPN_WORKS_KEYDESKS_SERVER_PORT
        export VPN_WORKS_KEYDESKS_SERVER_JUMPS

        # shellcheck source=/dev/null
        . /etc/vg-dc-mgmt/dc-name.env

        /usr/bin/flock -x -E 0 -n /tmp/vpn-works-keydesk-sync.lock "${basedir}"/vpn-works-keydesks-sync.sh 2>&1 | /usr/bin/logger -p local0.notice -t KDSYNC
}

if [ "addbrigade" = "${cmd}" ]; then
        "${basedir}"/addbrigade "$@"
        vpn_works_keysesks_sync
elif [ "delbrigade" = "${cmd}" ]; then
        "${basedir}"/delbrigade "$@"
        vpn_works_keysesks_sync
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
