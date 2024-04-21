#!/bin/sh

set -e

if [ -x "../restoresnaps" ]; then
        RESTORE="$(dirname "$0")/../restoresnaps"
elif go version >/dev/null 2>&1; then
        RESTORE="go run $(dirname "$0")/../"
else
        echo "No snap tool found"
        exit 1
fi

while [ $# -gt 0 ]; do
        case "$1" in
        *)
                echo "Unknown option: $1"
                exit 1
                ;;
        esac
done

DB_DIR=${DB_DIR:-"../../../../vpngen-keydesk/cmd/keydesk"}
DB_DIR="$(realpath "${DB_DIR}")"
CONF_DIR=${CONF_DIR:-"../../../../vpngen-keydesk-snap/core/crypto/testdata"}
CONF_DIR="$(realpath "${CONF_DIR}")"

MIGRATION_PLAN_FILE=${MIGRATION_PLAN_FILE:-"${DB_DIR}/migration-plan.json"}

if [ ! -s "${MIGRATION_PLAN_FILE}" ]; then
        echo "No migration plan found ${MIGRATION_PLAN_FILE}"
        exit 1
fi

REALM_PRIV_KEY_FILE=${REALM_PRIV_KEY_FILE:-"${CONF_DIR}/id_rsa_realm2-sample"}

if [ ! -s "${REALM_PRIV_KEY_FILE}" ]; then
        echo "No private key found in ${REALM_PRIV_KEY_FILE}"
        exit 1
fi

echo "Testing snapshot restore with migration plan"
echo "Using realm private key: ${REALM_PRIV_KEY_FILE}"
echo "Using migration plan: ${MIGRATION_PLAN_FILE}"
echo "Using reservation ID: 1b4c7f77-092c-4a9c-b38a-61b2aa699b73"

SSH_KEY="${HOME}/.ssh/id_ed25519" \
${RESTORE} -k "${REALM_PRIV_KEY_FILE}" -f "${MIGRATION_PLAN_FILE}" -r "1b4c7f77-092c-4a9c-b38a-61b2aa699b73"
