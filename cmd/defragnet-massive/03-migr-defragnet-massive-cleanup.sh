#!/bin/sh
#
# Massive brigade migration: destroy old brigade instances on source servers,
# remove old DB records, release the reservation.
# Run this ~24h after 02-migr-defragnet-massive-swith.sh (after DNS propagation).

set -e

DB_URL=${DB_URL:-"postgres:///vgrealm"}

mkdir -p "${HOME}/migr-logs"

print_usage() {
	echo "Usage: $0 -tag <tag>"
	echo "  -tag  the same TAG printed by 01-migr-defragnet-massive-propagade.sh"
	exit 1
}

TAG=""

while [ $# -gt 0 ]; do
	case "$1" in
		-tag) TAG="$2"; shift 2 ;;
		-h|--help) print_usage ;;
		*) echo "Unknown option: $1"; print_usage ;;
	esac
done

if [ -z "${TAG}" ]; then
	echo "ERROR: -tag is required"
	print_usage
fi

# ---------------------------------------------------------------------------
# locate files left by the propagade step
# ---------------------------------------------------------------------------

RESERVATION_FILE="$(ls "${HOME}/tmp/${TAG}-migr-reserv-"*.json 2>/dev/null | tail -n 1)"
if [ -z "${RESERVATION_FILE}" ] || [ ! -s "${RESERVATION_FILE}" ]; then
	echo "ERROR: no reservation file found in ~/tmp/ for tag '${TAG}'"
	exit 1
fi

RESERVATION="$(jq -r '.reservation_id' < "${RESERVATION_FILE}")"
if ! echo "${RESERVATION}" | grep -Eq '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'; then
	echo "ERROR: invalid UUID in ${RESERVATION_FILE}"
	exit 1
fi

PLAN_FILE="$(ls "${HOME}/tmp/${TAG}-migr-plan-"*.json 2>/dev/null | tail -n 1)"
if [ -z "${PLAN_FILE}" ] || [ ! -s "${PLAN_FILE}" ]; then
	echo "ERROR: no plan file found in ~/tmp/ for tag '${TAG}'"
	exit 1
fi

DTMARK="$(echo "${PLAN_FILE}" | sed 's/^.*\-migr\-plan\-//; s/\.json$//')"
PREPARED_FILE="${HOME}/tmp/${TAG}-migr-prepared-${DTMARK}.json"

if [ ! -s "${PREPARED_FILE}" ]; then
	echo "ERROR: prepared file not found: ${PREPARED_FILE}"
	exit 1
fi

echo "=== MASSIVE MIGRATION: CLEANUP ==="
echo "  TAG=${TAG}"
echo "  RESERVATION=${RESERVATION}"
echo "  PLAN_FILE=${PLAN_FILE}"
echo "  PREPARED_FILE=${PREPARED_FILE}"
echo

# ---------------------------------------------------------------------------
# STEP 1: purge — SSH-destroy old WireGuard brigade configs on source servers
# ---------------------------------------------------------------------------

echo ">>> STEP 1: PURGE OLD BRIGADE INSTANCES (SSH)"

/opt/vg-dc-snaps/switch_local_migr.sh purge \
	-r "${RESERVATION}" \
	-f "${PREPARED_FILE}" 2>&1 || \
	echo "WARNING: purge returned non-zero — some source-server configs may remain"

echo

# ---------------------------------------------------------------------------
# STEP 1b: carry stats forward — move the protection columns from the old
# (non-main) instances onto the new main BEFORE STEP 2 cascades them away.
#
# stats.brigades_stats is keyed (brigade_id, instance_id) and patch 012 hangs an
# ON DELETE CASCADE FK on brigades.brigades. A migrated brigade therefore starts
# a fresh stats row with peak_active_users and protected_until at zero, while its
# real history sits on the parked instance STEP 2 is about to remove. Until then
# the brigade is shielded by getwasted's "b2.brigade_id IS NULL" clause, so this
# is the exact moment that shield and the history disappear together.
#
# Only peak_active_users, peak_active_users_at and protected_until are worth
# copying: collectstats reassigns every other column from node state on each run,
# and the node's counters were reset by the restore. GREATEST ignores NULLs and
# can only raise a value, so this is idempotent and deliberately not scoped to
# this batch — running it fleet-wide on every cleanup is harmless and also
# repairs batches cleaned up before this step existed.
# ---------------------------------------------------------------------------

echo ">>> STEP 1b: CARRY STATS FORWARD TO NEW INSTANCES"

# Needs sql/patches/039-stats-migr-carryforward-grant.sql applied: 021 gave the
# migration role only SELECT/INSERT/DELETE on the stats schema, so without 039
# this fails with "permission denied for table brigades_stats".
psql "${DB_URL}" -q <<'EOSQL' || echo "WARNING: stats carry-forward returned non-zero"
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

echo

# ---------------------------------------------------------------------------
# STEP 2: delete — remove old (non-main) brigade DB records
# ---------------------------------------------------------------------------

echo ">>> STEP 2: DELETE OLD DB RECORDS"

/opt/vg-dc-snaps/switch_local_migr.sh delete \
	-r "${RESERVATION}" \
	-f "${PREPARED_FILE}" 2>&1 || \
	echo "WARNING: delete returned non-zero — check DB for orphaned records"

echo

# ---------------------------------------------------------------------------
# STEP 3: release reservation
# ---------------------------------------------------------------------------

echo ">>> STEP 3: RELEASE RESERVATION"

/opt/vg-dc-snaps/create_reservation.sh delete -f "${RESERVATION}" || \
	echo "WARNING: reservation delete returned non-zero"

echo

echo "=== CLEANUP COMPLETE ==="
echo "  Old brigade instances removed from source servers."
echo "  Reservation ${RESERVATION} released."
echo
echo "Next step (admin): run ./02-admin-defragnet-massive-cleanup.sh <source-networks...>"
echo "  to remove the source endpoint IPs from the pairs table."
