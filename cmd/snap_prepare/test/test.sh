#!/bin/sh

set -e

if [ -x "../snap_prepare" ]; then
        PREPARE="$(dirname "$0")/../snap_prepare"
elif go version >/dev/null 2>&1; then
        PREPARE="go run $(dirname "$0")/../"
else
        echo "No snap tool found"
        exit 1
fi

FORCE=${FORCE:-""}

while [ $# -gt 0 ]; do
        case "$1" in
        -force)
                FORCE="$1"
                ;;
        -fp)
                AUTHORITY_FP="$2"
                shift
                ;;
        *)
                echo "Unknown option: $1"
                exit 1
                ;;
        esac
        shift
done

DB_DIR=${DB_DIR:-"../../../../vpngen-keydesk/cmd/keydesk"}
DB_DIR="$(realpath "${DB_DIR}")"
CONF_DIR=${CONF_DIR:-"../../../../vpngen-keydesk-snap/core/crypto/testdata"}
CONF_DIR="$(realpath "${CONF_DIR}")"

SNAPSHOT_FILE=${SNAPSHOT_FILE:-"${DB_DIR}/brigades.snapshot.test.json"}

if [ ! -s "${SNAPSHOT_FILE}" ]; then
        echo "No keydesk snapshot found ${SNAPSHOT_FILE}"
        exit 1
fi

REALM_PRIV_KEY_FILE=${REALM_PRIV_KEY_FILE:-"${CONF_DIR}/id_rsa_realm1-sample"}

if [ ! -s "${REALM_PRIV_KEY_FILE}" ]; then
        echo "No private key found in ${REALM_PRIV_KEY_FILE}"
        exit 1
fi

AUTHORITIES_FILE=${AUTHORITIES_FILE:-"${CONF_DIR}/authorities_keys"}

if [ ! -s "${AUTHORITIES_FILE}" ]; then
        echo "No authorities keys found in ${AUTHORITIES_FILE}"
        exit 1
fi

AUTHORITY_FP=${AUTHORITY_FP:-"SHA256:$(grep "ssh-rsa" "${AUTHORITIES_FILE}" | head -n 1 | awk '{print $2}' | base64 -d | openssl dgst -sha256 -binary | base64 -w 0 | sed 's/=//g' | awk '{print $1}' )"}

echo "Testing snapshot reencryption"
echo "Using authority fingerprint: ${AUTHORITY_FP}"
echo "Using realm private key: ${REALM_PRIV_KEY_FILE}"
echo "Using authorities file: ${AUTHORITIES_FILE}"

${PREPARE} -fp "${AUTHORITY_FP}" -a "${AUTHORITIES_FILE}" -k "${REALM_PRIV_KEY_FILE}" "${FORCE}" < "${SNAPSHOT_FILE}" | tee "${DB_DIR}/brigade.snapshot.reencrypt.json"
