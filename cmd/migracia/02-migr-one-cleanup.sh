#!/bin/sh

set -e

DB_URL=${DB_URL:-"postgres:///vgrealm"}

if [ ! -d "${HOME}/migr-logs" ]; then
        mkdir -p "${HOME}/migr-logs"
fi

if [ ! -d "${HOME}/tmp" ]; then
        mkdir -p "${HOME}/tmp"
fi

while [ $# -gt 0 ]; do
        case "$1" in
                -h|--help)
                        echo "Usage: $0 -ip <external-ip>  [-target <target_cidr>] [-ctrl <infranet_cidr>]"
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

echo "SWITCH CLEANUP LOCAL"
echo "NETWORK=${NETWORK} TARGETNET=${TARGETNET} BASENET=${BASENET}"
echo "RESERVATION: ${RESERVATION}"
echo "DTMARK: ${DTMARK}"
echo "PLAN_FILE: ${PLAN_FILE}"
echo "PREPARED_FILE: ${PREPARED_FILE}"

echo "/opt/vg-dc-snaps/switch_local_migr.sh purge -r \"${RESERVATION}\" -f \"${PREPARED_FILE}\""
/opt/vg-dc-snaps/switch_local_migr.sh  purge -r "${RESERVATION}" -f "${PREPARED_FILE}" || \
	echo "!!! SWITCH CLEANUP LOCAL FAILED: ${PREPARED_FILE}"

# Carry the protection columns from the old (non-main) instances onto the new
# main before the delete below cascades them away. See the long note in
# cmd/defragnet-massive/03-migr-defragnet-massive-cleanup.sh. GREATEST ignores
# NULLs and only ever raises a value, so this is idempotent and intentionally
# not scoped to this brigade.
echo "CARRY STATS FORWARD TO NEW INSTANCES"
# Needs sql/patches/039-stats-migr-carryforward-grant.sql applied.
psql "${DB_URL}" -q <<'EOSQL' || echo "!!! STATS CARRY-FORWARD FAILED"
UPDATE stats.brigades_stats sm
SET peak_active_users    = GREATEST(sm.peak_active_users,    agg.peak),
    peak_active_users_at = GREATEST(sm.peak_active_users_at, agg.peak_at),
    protected_until      = GREATEST(sm.protected_until,      agg.prot)
FROM brigades.brigades bm,
LATERAL (
        SELECT max(so.peak_active_users)    AS peak,
               max(so.peak_active_users_at) AS peak_at,
               max(so.protected_until)      AS prot
        FROM brigades.brigades bo
        JOIN stats.brigades_stats so USING (brigade_id, instance_id)
        WHERE bo.brigade_id = bm.brigade_id AND bo.main = false
) agg
WHERE sm.brigade_id  = bm.brigade_id
  AND sm.instance_id = bm.instance_id
  AND bm.main = true
  AND agg.peak IS NOT NULL;
EOSQL

echo "/opt/vg-dc-snaps/switch_local_migr.sh delete -r \"${RESERVATION}\" -f \"${PREPARED_FILE}\""
/opt/vg-dc-snaps/switch_local_migr.sh  delete -r "${RESERVATION}" -f "${PREPARED_FILE}" || \
	echo "!!! SWITCH CLEANUP LOCAL FAILED: ${PREPARED_FILE}"

echo "/opt/vg-dc-snaps/create_reservation.sh delete -f \"${RESERVATION}\""
/opt/vg-dc-snaps/create_reservation.sh delete -f "${RESERVATION}" || \
	echo "!!! RESERVATION CLEANUP FAILED: ${PREPARED_FILE}"

echo "CLEANUP LOCAL COMPLETE: ${PREPARED_FILE}" 


