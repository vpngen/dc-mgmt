#!/bin/sh

set -e

if [ ! -d "${HOME}/migr-logs" ]; then
        mkdir -p "${HOME}/migr-logs"
fi

if [ ! -d "${HOME}/tmp" ]; then
        mkdir -p "${HOME}/tmp"
fi

NETWORK=$1
INFRANET=$2

if [ -z "${NETWORK}" ] || [ -z "${INFRANET}" ]; then
        echo "no network"
        exit 1
fi

if [ -z "${NETWORK}" ]; then
        NETWORK="0.0.0.0/0"
fi

if [ -z "${INFRANET}" ]; then
        INFRANET="0.0.0.0/0"
fi

BASENET_INFRANET=${INFRANET%%/*}
BASENET_NETWORK=${NETWORK%%/*}

BASENET="${BASENET_NETWORK}-${BASENET_INFRANET}"
                                                                                                                                                                                                                 echo "NETWORK=${NETWORK} INFRANET=${INFRANET} BASENET=${BASENET}" 

RESERVATION_FILE="$(ls ${HOME}/tmp/${BASENET}-migr-reserv-*.json | tail -n 1)"
if [ -z "${RESERVATION_FILE}" ] || [ ! -s "${RESERVATION_FILE}" ]; then
	echo "no reservation"
	exit 1
fi

RESERVATION="$(jq -r '.reservation_id' < "${RESERVATION_FILE}")"
if ! echo "$RESERVATION" | grep -Eq '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'; then
    echo "Invalid UUID"
    exit 1
fi

PLAN_FILE="$(ls ${HOME}/tmp/${BASENET}-migr-plan-*.json | tail -n 1)"
if [ -z "${PLAN_FILE}" ] || [ ! -s "${PLAN_FILE}" ]; then
	echo "no plan"
	exit 1
fi

DTMARK="$(echo "${PLAN_FILE}" | sed 's/^.*\-migr\-plan\-//; s/\.json$//')"

PREPARED_FILE="${HOME}/tmp/${BASENET}-migr-prepared-${DTMARK}.json"
if [ -z "${PREPARED_FILE}" ] || [ ! -s "${PREPARED_FILE}" ]; then
	echo "no prepared"
	exit 1
fi

echo "SWKTCH LOCAL"
echo "NETWORK=${NETWORK} BASENET=${BASENET}"
echo "RESERVATION: ${RESERVATION}"
echo "DTMARK: ${DTMARK}"
echo "PLAN_FILE: ${PLAN_FILE}"
echo "PREPARED_FILE: ${PREPARED_FILE}"

/opt/vg-dc-snaps/switch_local_migr.sh switch -r "${RESERVATION}" -f "${PREPARED_FILE}" || \
	echo "!!! SWITCH LOCAL FAILED: ${PREPARED_FILE}"

sudo -u vgvpnapi SSH_KEY=/home/vgvpnapi/.ssh/id_ed25519 /opt/vg-dc-vpnapi/delegation-sync.sh || \
	echo "!!! SWITCH LOCAL DELEGATION SYNC FAILED: ${PREPARED_FILE}"

echo "SWITCH LOCAL COMPLETE: ${PREPARED_FILE}"

