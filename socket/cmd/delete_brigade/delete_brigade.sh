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

basedir=$(dirname "$0")

if [ -s  "/etc/vg-dc-mgmt/dc-name.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-mgmt/dc-name.env"
fi

if [ -s "/etc/vg-dc-vpnapi/modbrigade.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-vpnapi/modbrigade.env"
fi

DC_ID="${DC_ID}" \
DC_NAME="${DC_NAME}" \
SUBDOMAIN_API_SERVER="${SUBDOMAIN_API_SERVER}" \
SUBDOMAIN_API_TOKEN="${SUBDOMAIN_API_TOKEN}" \
DELEGATION_SYNC_CONNECT="${DELEGATION_SYNC_CONNECT}" \
MANAGEMENT_RANDOM_RESPONSES="${MANAGEMENT_RANDOM_RESPONSES}" \
flock -x -E 1 -w 60 /tmp/modbrigade.lock "${basedir}"/delete_brigade "$@"
