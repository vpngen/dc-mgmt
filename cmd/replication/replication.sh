#!/bin/sh

if [ -s "${HOME}/.secret/local_migration.env" ]; then
        # shellcheck source=/dev/null
        . "${HOME}/.secret/local_migration.env"
fi

set -e

if [ -z "${AUTHFP}" ]; then
        echo "AUTHFP not set"
        exit 1
fi

if [ -z "${REALM_FP}" ]; then
        echo "REALM_FP not set"
        exit 1
fi

if [ "$#" -eq 0 ]; then
        for repl in $(/opt/vg-dc-snaps/create_replication.sh autolist); do
                replication_id=$(echo "${repl}" | cut -f 1 -d '|')
                infranet=$(echo "${repl}" | cut -f 2 -d '|')
                network=$(echo "${repl}" | cut -f 3 -d '|')

                if [ -z "${replication_id}" ] || [ -z "${infranet}" ] || [ -z "${network}" ]; then
                        echo "no replication id or infranet or network"
                        continue
                fi

                # shellcheck disable=SC2046
                flock -x -n /tmp/replication-"${replication_id}".lock "$0" "${replication_id}" "${network}" "${infranet}"
        done

        exit 0
fi


if [ "$#" -ne 3 ]; then
        echo "Usage: $0 <replication> <network> <infranet>"
        exit 1
fi

REPLICATION=$1
NETWORK=$2
INFRANET=$3

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

echo "REPLICATION=${REPLICATION}"
echo "NETWORK=${NETWORK} INFRANET=${INFRANET} BASENET=${BASENET}"

sudo -u vgsnaps /opt/vg-dc-snaps/collectsnaps.sh -tag "${BASENET}-replica" -net "${NETWORK}" -ctrl "${INFRANET}" -ad -r
jq -r '"errors_count=\(.errors_count), total_count=\(.total_count)"' < "$(ls /vg-snapshots/${BASENET}-replica/${BASENET}*.json | tail -n 1)"
echo "$(ls /vg-snapshots/${BASENET}-replica/${BASENET}*.json | tail -n 1)"

SNAPSHOT=${SNAPSHOT:-"$(ls /vg-snapshots/${BASENET}-replica/${BASENET}*.json | tail -n 1)"}
if [ -z "${SNAPSHOT}" ] || [ ! -s "${SNAPSHOT}" ]; then
        echo "no snapshot"
        exit 1
fi

DTMARK="$(basename "${SNAPSHOT}" | sed 's/^.*\-replica\-//; s/\.json$//')"

echo "SNAPSHOT=${SNAPSHOT}"
echo "REPLICATION=${REPLICATION}"

reserv="$(/opt/vg-dc-snaps/create_replication.sh autolist 2>&1 | grep "${REPLICATION}" | cut -f 1 -d '|')"

if ! echo "$reserv" | grep -Eq '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'; then
    echo "Invalid UUID"

    exit 1
fi

rshort="${reserv%%-*}"

echo "REPLICATION: ${NETWORK} ${INFRANET} $reserv ($rshort)"

if [ ! -d "${HOME}/tmp" ]; then
        mkdir -p "${HOME}/tmp"
fi

REPLICATION_CONFIG_FILE="${HOME}/tmp/${BASENET}-replica-conf-$rshort.json"
REPLICATION_MAP_FILE="${HOME}/tmp/${BASENET}-replica-map-$rshort.json"

/opt/vg-dc-snaps/create_replication.sh genconf "$reserv" > "${REPLICATION_CONFIG_FILE}"
/opt/vg-dc-snaps/create_replication.sh genmap "$reserv" > "${REPLICATION_MAP_FILE}"

PREPARED_FILE="${HOME}/tmp/${BASENET}-replica-prepared-${DTMARK}.json"

/opt/vg-dc-snaps/snap_prepare -fp "${AUTHFP}" < "${SNAPSHOT}" > "${PREPARED_FILE}"

echo "PREPARE COMPLETE: ${REPLICATION_CONFIG_FILE} | ${REPLICATION_MAP_FILE} | ${PREPARED_FILE}"

PLAN_FILE="${HOME}/tmp/${BASENET}-replica-plan-${DTMARK}.json"

/opt/vg-dc-snaps/recodesnaps -tfp "${REALM_FP}" -c "${REPLICATION_CONFIG_FILE}" -rkeys "/etc/vg-dc-snaps/realms_keys" -map "${REPLICATION_MAP_FILE}" -in "${PREPARED_FILE}" -out "${PLAN_FILE}"

if [ -f "${REPLICATION_MAP_FILE}" ]; then
        echo "REMOVE MAP FILE: ${REPLICATION_MAP_FILE}"
        rm -f "${REPLICATION_MAP_FILE}"
fi

if [ -f "${REPLICATION_CONFIG_FILE}" ]; then
        echo "REMOVE CONFIG FILE: ${REPLICATION_CONFIG_FILE}"
        rm -f "${REPLICATION_CONFIG_FILE}"
fi

if [ -f "${PREPARED_FILE}" ]; then
        echo "REMOVE PREPARED FILE: ${PREPARED_FILE}"
        rm -f "${PREPARED_FILE}"
fi

echo "RECODE COMPLETE: ${PLAN_FILE}"
#echo "/opt/vg-dc-snaps/restoresnaps -p -r ${reserv} -f ${PLAN_FILE} 2>&1 | tee log-replication-${BASENET}-replica-plan-${DTMARK}-$(date +"%Y%m%d%H%M").log"

/opt/vg-dc-snaps/restoresnaps -p -r "${reserv}" -f "${PLAN_FILE}" 2>&1 | tee "log-replication-${BASENET}-replica-plan-${DTMARK}-$(date +"%Y%m%d%H%M").log"

if [ -f "${PLAN_FILE}" ]; then
        echo "REMOVE PLAN FILE: ${PLAN_FILE}"
        rm -f "${PLAN_FILE}"
fi