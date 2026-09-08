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

while [ $# -gt 0 ]; do
        case "$1" in
                -h|--help)
                        echo "Usage: $0 -ip <external-ip> [-target <target_cidr>] [-ctrl <infranet_cidr>]"
                        exit 0
                        ;;
                -ip)
                        NETWORK="$2/32"
                        shift 2
                        ;;
                -ctrl)
                        INFRANET="$2"
                        shift 2
                        ;;
                -target)
                        TARGETNET="$2"
                        shift 2
                        ;;
                *)
                        break
                        ;;
        esac
done

COUNT=1

if [ -z "${NETWORK}" ]; then
        echo "no network"
        exit 1
fi

if [ -z "${TARGETNET}" ]; then
        TARGETNET="0.0.0.0/0"
fi

if [ -z "${INFRANET}" ]; then
        INFRANET="10.30.0.0/16"
fi

BASENET_INFRANET=${INFRANET%%/*}
BASENET_NETWORK=${NETWORK%%/*}
BASENET_TARGETNET=${TARGETNET%%/*}

BASENET="${BASENET_NETWORK}-${BASENET_INFRANET}-${BASENET_TARGETNET}"

echo "NETWORK=${NETWORK} INFRANET=${INFRANET} TARGETNET=${TARGETNET} BASENET=${BASENET}"

MNT_TO=${MNT_TO:-"$(date -d '+7 day' +%s)"}

if [ -d "/vg-snapshots/${BASENET}-migr/" ]; then
        SNAPSHOT=${SNAPSHOT:-"$(ls /vg-snapshots/${BASENET}-migr/${BASENET}*.json | tail -n 1)"}
        if [ -n "${SNAPSHOT}" ] && [ -s "${SNAPSHOT}" ]; then
                echo "existing snapshot: ${SNAPSHOT}"
        fi
fi

if [ -z "${SNAPSHOT}" ] || [ ! -s "${SNAPSHOT}" ]; then
        echo "sudo -u vgsnaps /opt/vg-dc-snaps/collectsnaps.sh -tag \"${BASENET}-migr\" -net \"${NETWORK}\" -ad -mnt \"${MNT_TO}\""
        sudo -u vgsnaps /opt/vg-dc-snaps/collectsnaps.sh -tag "${BASENET}-migr" -net "${NETWORK}" -ad -mnt "${MNT_TO}"
        jq -r '"errors_count=\(.errors_count), total_count=\(.total_count)"' < "$(ls /vg-snapshots/${BASENET}-migr/${BASENET}*.json | tail -n 1)"
        echo "$(ls /vg-snapshots/${BASENET}-migr/${BASENET}*.json | tail -n 1)"
fi

SNAPSHOT=${SNAPSHOT:-"$(ls /vg-snapshots/${BASENET}-migr/${BASENET}*.json | tail -n 1)"}
if [ -z "${SNAPSHOT}" ] || [ ! -s "${SNAPSHOT}" ]; then
        echo "no snapshot"
        exit 1
fi

DTMARK="$(basename "${SNAPSHOT}" | sed 's/^.*\-migr\-//; s/\.json$//')"

echo "BASENET=${BASENET} NETWORK=${NETWORK} TARGETNET=${TARGETNET} INFRANET=${INFRANET}"
echo "SNAPSHOT=${SNAPSHOT}"
echo "DTMARK=${DTMARK}"
echo "COUNT=${COUNT}"


RESERVATION_CONFIG_FILE="$(ls ${HOME}/tmp/${BASENET}-migr-reserv-*.json | tail -n 1)"
if [ -n "${RESERVATION_CONFIG_FILE}" ] && [ -s "${RESERVATION_CONFIG_FILE}" ]; then
        RESERVATION="$(jq -r '.reservation_id' < "${RESERVATION_CONFIG_FILE}")"
        if [ -n "$RESERVATION" ]; then
                if ! echo "$RESERVATION" | grep -Eq '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'; then
                        echo "Invalid UUID"
                        exit 1
                fi
        fi
fi


if [ -z "${RESERVATION_CONFIG_FILE}" ] || [ ! -s "${RESERVATION_CONFIG_FILE}" ] || [ -z "$RESERVATION" ]; then
        echo "RESERVATION CREATE..."
        echo "/opt/vg-dc-snaps/create_reservation.sh create -nc -in \"${INFRANET}\" -en \"${TARGETNET}\" \"$COUNT\""

        RESERVATION="$(/opt/vg-dc-snaps/create_reservation.sh create -nc -in "${INFRANET}" "$COUNT" 2>&1 | grep "Created reservation:" | cut -f 3 -d ' ')"
        if ! echo "${RESERVATION}" | grep -Eq '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'; then
                echo "Invalid UUID"

                exit 1
        fi

        rshort="${RESERVATION%%-*}"

        echo "RESERVATION: ${NETWORK} ${RESERVATION} ($rshort) ${COUNT}"

        RESERVATION_CONFIG_FILE="${HOME}/tmp/${BASENET}-migr-reserv-${rshort}.json"

        echo "/opt/vg-dc-snaps/create_reservation.sh genconf \"${RESERVATION}\" > \"${RESERVATION_CONFIG_FILE}\""
        /opt/vg-dc-snaps/create_reservation.sh genconf "${RESERVATION}" > "${RESERVATION_CONFIG_FILE}"
fi

set -e

PREPARED_FILE="${HOME}/tmp/${BASENET}-migr-prepared-${DTMARK}.json"

if [ ! -s "${PREPARED_FILE}" ]; then
        echo "/opt/vg-dc-snaps/snap_prepare -fp \"${AUTHFP}\" < \"${SNAPSHOT}\" > \"${PREPARED_FILE}\""
        /opt/vg-dc-snaps/snap_prepare -fp "${AUTHFP}" < "${SNAPSHOT}" > "${PREPARED_FILE}"
fi

echo "PREPARE COMPLETE: ${RESERVATION_CONFIG_FILE} | ${PREPARED_FILE}"

PLAN_FILE="${HOME}/tmp/${BASENET}-migr-plan-${DTMARK}.json"

if [ ! -s "${PLAN_FILE}" ]; then
        echo "/opt/vg-dc-snaps/recodesnaps -tfp \"${REALM_FP}\" -c \"${RESERVATION_CONFIG_FILE}\" -rkeys \"/etc/vg-dc-snaps/realms_keys\" -in \"${PREPARED_FILE}\" -out \"${PLAN_FILE}\""
        /opt/vg-dc-snaps/recodesnaps -tfp "${REALM_FP}" -c "${RESERVATION_CONFIG_FILE}" -rkeys "/etc/vg-dc-snaps/realms_keys" -in "${PREPARED_FILE}" -out "${PLAN_FILE}"
fi

echo "RECODE COMPLETE: ${PLAN_FILE}"

echo "/opt/vg-dc-snaps/restoresnaps -r \"${RESERVATION}\" -f \"${PLAN_FILE}\" 2>&1"
/opt/vg-dc-snaps/restoresnaps -r "${RESERVATION}" -f "${PLAN_FILE}" 2>&1 # | tee "${HOME}/migr-logs/$(date +"%Y%m%d%H%M")-${BASENET}-migr-plan-${DTMARK}.log"


echo "SWKTCH LOCAL"
echo "NETWORK=${NETWORK} BASENET=${BASENET} TARGETNET=${TARGETNET}"
echo "RESERVATION: ${RESERVATION}"
echo "DTMARK: ${DTMARK}"
echo "PLAN_FILE: ${PLAN_FILE}"
echo "PREPARED_FILE: ${PREPARED_FILE}"

echo "/opt/vg-dc-snaps/switch_local_migr.sh switch -r \"${RESERVATION}\" -f \"${PREPARED_FILE}\""
/opt/vg-dc-snaps/switch_local_migr.sh switch -r "${RESERVATION}" -f "${PREPARED_FILE}" || \
        echo "!!! SWITCH LOCAL FAILED: ${PREPARED_FILE}"

echo sudo -u vgvpnapi SSH_KEY=/home/vgvpnapi/.ssh/id_ed25519 /opt/vg-dc-vpnapi/delegation-sync.sh
sudo -u vgvpnapi SSH_KEY=/home/vgvpnapi/.ssh/id_ed25519 /opt/vg-dc-vpnapi/delegation-sync.sh || \
        echo "!!! SWITCH LOCAL DELEGATION SYNC FAILED: ${PREPARED_FILE}"

echo "SWITCH LOCAL COMPLETE: ${PREPARED_FILE}"