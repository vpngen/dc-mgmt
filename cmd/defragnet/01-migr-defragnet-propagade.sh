#!/bin/sh

if [ -s "${HOME}/.secret/local_migration.env" ]; then
        # shellcheck source=/dev/null
        . "${HOME}/.secret/local_migration.env"
fi
 
set -e                                                                                                                                                                                                           

if [ ! -d "${HOME}/migr-logs" ]; then
        mkdir -p "${HOME}/migr-logs"
fi

if [ ! -d "${HOME}/tmp" ]; then
        mkdir -p "${HOME}/tmp"
fi

if [ -z "${AUTHFP}" ]; then
        echo "AUTHFP not set"
        exit 1
fi

if [ -z "${REALM_FP}" ]; then
        echo "REALM_FP not set"
fi

NETWORK=$1
INFRANET=$2
THIRD=$3

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

MNT_TO=${MNT_TO:-"$(date -d '+7 day' +%s)"}

sudo -u vgsnaps /opt/vg-dc-snaps/collectsnaps.sh -tag "${BASENET}-migr" -net "${NETWORK}" -ctrl "${INFRANET}" -ad -mnt "${MNT_TO}"
jq -r '"errors_count=\(.errors_count), total_count=\(.total_count)"' < "$(ls /vg-snapshots/${BASENET}-migr/${BASENET}*.json | tail -n 1)"
echo "$(ls /vg-snapshots/${BASENET}-migr/${BASENET}*.json | tail -n 1)"

SNAPSHOT=${SNAPSHOT:-"$(ls /vg-snapshots/${BASENET}-migr/${BASENET}*.json | tail -n 1)"}
if [ -z "${SNAPSHOT}" ] || [ ! -s "${SNAPSHOT}" ]; then
	echo "no snapshot"
	exit 1
fi

DTMARK="$(basename "${SNAPSHOT}" | sed 's/^.*\-migr\-//; s/\.json$//')"
COUNT=${THIRD:-"$(jq -r '"\(.total_count)"' < "${SNAPSHOT}")"}

echo "BASENET=${BASENET} NETWORK=${NETWORK} INFRANET=${INFRANET}"
echo "SNAPSHOT=${SNAPSHOT}"
echo "DTMARK=${DTMARK}"
echo "COUNT=${COUNT}"

reserv="$(/opt/vg-dc-snaps/create_reservation.sh create -nc -in "${INFRANET}" "$COUNT" 2>&1 | grep "Created reservation:" | cut -f 3 -d ' ')"

if ! echo "$reserv" | grep -Eq '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'; then
    echo "Invalid UUID"

    exit 1
fi

rshort="${reserv%%-*}"

echo "RESERVATION: ${NETWORK} $reserv ($rshort) ${COUNT}"

RESERVATION_CONFIG_FILE="${HOME}/tmp/${BASENET}-migr-reserv-${rshort}.json"

/opt/vg-dc-snaps/create_reservation.sh genconf "$reserv" > "${RESERVATION_CONFIG_FILE}"

PREPARED_FILE="${HOME}/tmp/${BASENET}-migr-prepared-${DTMARK}.json"

/opt/vg-dc-snaps/snap_prepare -fp "${AUTHFP}" < "${SNAPSHOT}" > "${PREPARED_FILE}"

echo "PREPARE COMPLETE: ${RESERVATION_CONFIG_FILE} | ${PREPARED_FILE}"

PLAN_FILE="${HOME}/tmp/${BASENET}-migr-plan-${DTMARK}.json"

/opt/vg-dc-snaps/recodesnaps -tfp "${REALM_FP}" -c "${RESERVATION_CONFIG_FILE}" -rkeys "/etc/vg-dc-snaps/realms_keys" -in "${PREPARED_FILE}" -out "${PLAN_FILE}"

#if [ -f "${RESERVATION_CONFIG_FILE}" ]; then
#        echo "REMOVE CONFIG FILE: ${RESERVATION_CONFIG_FILE}"
#        rm -f "${RESERVATION_CONFIG_FILE}"
#fi

#if [ -f "${PREPARED_FILE}" ]; then
#        echo "REMOVE PREPARED FILE: ${PREPARED_FILE}"
#        rm -f "${PREPARED_FILE}"
#fi

echo "RECODE COMPLETE: ${PLAN_FILE}"

/opt/vg-dc-snaps/restoresnaps -r "${reserv}" -f "${PLAN_FILE}" 2>&1 | tee "${HOME}/migr-logs/$(date +"%Y%m%d%H%M")-${BASENET}-migr-plan-${DTMARK}.log"

#if [ -f "${PLAN_FILE}" ]; then
#        echo "REMOVE PLAN FILE: ${PLAN_FILE}"
#        rm -f "${PLAN_FILE}"
#fi

