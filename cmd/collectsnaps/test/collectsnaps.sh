#!/bin/sh

printdef() {
    echo "Usage: [-tag <tag>]"
    exit 1
}

basedir=$(dirname "$0")

DB_DIR=${DB_DIR:-"../../../../vpngen-keydesk/cmd/keydesk"}
DB_DIR="$(realpath "${DB_DIR}")"
CONF_DIR=${CONF_DIR:-"../../../../vpngen-keydesk-snap/core/crypto/testdata"}
CONF_DIR="$(realpath "${CONF_DIR}")"

DC_ID=${DC_ID:-"b8185201-7dbe-4b45-b66a-d3ada82a9f34"} \
DC_NAME=${DC_NAME:-"test"} 

if [ ! -s "${CONF_DIR}/realms_keys" ]; then
        echo "No realms keys found in ${CONF_DIR}"
        exit 1
fi

REALM_FP=${REALM_FP:-"SHA256:$(grep "ssh-rsa" "${CONF_DIR}/realms_keys" | head -n 1 | awk '{print $2}' | base64 -d | openssl dgst -sha256 -binary | base64 -w 0 | sed 's/=//g' | awk '{print $1}' )"}

if [ ! -s "${CONF_DIR}/authorities_keys" ]; then
        echo "No authorities keys found in ${CONF_DIR}"
        exit 1
fi

REALMS_KEYS_PATH="${CONF_DIR}" 
SNAPSHOTS_BASE_DIR="${DB_DIR}" 


TESTAPP="go run ${basedir}"

DC_ID="${DC_ID}" \
DC_NAME="${DC_NAME}" \
REALM_FP="${REALM_FP}" \
REALMS_KEYS_PATH="${REALMS_KEYS_PATH}" \
SNAPSHOTS_BASE_DIR="${SNAPSHOTS_BASE_DIR}" \
$TESTAPP "$@"
