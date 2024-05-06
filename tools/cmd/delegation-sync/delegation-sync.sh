#!/bin/sh

CONF_DIR=${CONF_DIR:-"${HOME}"}

if [ -s "/etc/vg-dc-vpnapi/modbrigade.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-vpnapi/modbrigade.env"
fi

if [ -s  "/etc/vg-dc-mgmt/dc-name.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-mgmt/dc-name.env"
fi

basedir=$(dirname "$0")

SSH_KEY=${SSH_KEY:-"${CONFDIR}/.ssh/id_ed25519"} \
BRIGADES_SCHEMA=${BRIGADES_SCHEMA:-"brigades"} \
DB_URL=${DB_URL:-"postgres:///vgrealm"} \
DC_ID="${DC_ID}" \
DC_NAME="${DC_NAME}" \
DELEGATION_SYNC_CONNECT="${DELEGATION_SYNC_CONNECT}" \
flock -x -E 1 -w 60 /tmp/modbrigade.lock "${basedir}"/delegation-sync

exit 0
