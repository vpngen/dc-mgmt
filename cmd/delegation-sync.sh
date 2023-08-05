#!/bin/sh

CONF_DIR=${CONF_DIR:-"${HOME}"}

DBNAME=${DBNAME:-"vgrealm"}
SCHEMA=${SCHEMA:-"brigades"}
SSH_KEY=${SSH_KEY:-"${CONFDIR}/.ssh/id_ed25519"}

if [ -s "/etc/vg-dc-vpnapi/delegations-sync.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-vpnapi/delegations-sync.env"
fi

if [ -s  "/etc/vg-dc-mgmt/dc-name.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-mgmt/dc-name.env"
fi

WORK_DIR=${WORK_DIR:-"${HOME}"}

DELEGATION_FILE="${WORK_DIR}/domain-generate-${DC_NAME}.csv"
echo "Delegation file: ${DELEGATION_FILE}" 2>&1

TEMP_DELEGATION_FILE="${DELEGATION_FILE}.tmp"
RELOAD_FILE="${WORK_DIR}/domain-generate.reload"

DELEGATION_SYNC_SERVER_ADDR=${DELEGATION_SYNC_SERVER_ADDR:-$(cat "${CONF_DIR}/delegation_sync_server")}
DELEGATION_SYNC_SERVER_PORT=${DELEGATION_SYNC_SERVER_PORT:-"22"}
DELEGATION_SYNC_SERVER_JUMPS=${DELEGATION_SYNC_SERVER_JUMPS:-""} # separated by commas

SSH_KEY=${SSH_KEY:-"${CONF_DIR}/.ssh/id_ed25519"}
REMOTE_DELEGATION_FILE=${REMOTE_DELEGATION_FILE:-"~/domain-generate-${DC_NAME}.csv"}
REMOTE_RELOAD_FILE=${REMOTE_RELOAD_FILE:-"~/domain-generate.reload"}

jumps=""
if [ -n "${STATS_SYNC_SERVER_JUMPS}" ]; then
        jumps="-J ${STATS_SYNC_SERVER_JUMPS}"
fi

psql -qtAF ';' -d "${DBNAME}" \
        --set brigades_schema_name="${PAIRS_SCHEMA}" <<EOF | sed 's/;$//g' > "${TEMP_DELEGATION_FILE}"
BEGIN;

SELECT domain_name,endpoint_ipv4 FROM :"brigades_schema_name".domains_endpoints_ipv4;

ROLLBACK;
EOF

mv -f "${TEMP_DELEGATION_FILE}" "${DELEGATION_FILE}"

rsync -e "ssh -p ${DELEGATION_SYNC_SERVER_PORT} -i ${SSH_KEY} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null ${jumps}" \
        -aq \
        "${DELEGATION_FILE}" \
        "${DELEGATION_SYNC_SERVER_ADDR}:${REMOTE_DELEGATION_FILE}"

touch "${RELOAD_FILE}"

rsync -e "ssh -p ${DELEGATION_SYNC_SERVER_PORT} -i ${SSH_KEY} -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null ${jumps}" \
        -aq \
        "${RELOAD_FILE}" \
        "${DELEGATION_SYNC_SERVER_ADDR}:${REMOTE_RELOAD_FILE}"

exit 0
