#!/bin/sh
#
# Massive brigade migration: switch DB records to new instances + sync DNS.
# Run this after 01-migr-defragnet-massive-propagade.sh succeeds.
# Next step (after ~24h DNS propagation): 03-migr-defragnet-massive-cleanup.sh -tag <TAG>

set -e

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

echo "=== MASSIVE MIGRATION: SWITCH ==="
echo "  TAG=${TAG}"
echo "  RESERVATION=${RESERVATION}"
echo "  PLAN_FILE=${PLAN_FILE}"
echo "  PREPARED_FILE=${PREPARED_FILE}"
echo

# ---------------------------------------------------------------------------
# STEP 1: switch DB records — spare instances become main
# ---------------------------------------------------------------------------

echo ">>> STEP 1: SWITCH LOCAL"

SWITCH_LOG="${HOME}/migr-logs/$(date +"%Y%m%d%H%M")-${TAG}-switch-${DTMARK}.log"
START_TIME="$(date +%s)"

/opt/vg-dc-snaps/switch_local_migr.sh switch \
	-r "${RESERVATION}" \
	-f "${PREPARED_FILE}" 2>&1 | tee "${SWITCH_LOG}" || \
	echo "WARNING: switch_local_migr.sh returned non-zero"

END_TIME_SWITCH="$(date +%s)"

# ---------------------------------------------------------------------------
# STEP 2: push updated domain→IP mappings to DNS
# ---------------------------------------------------------------------------

echo
echo ">>> STEP 2: DELEGATION SYNC"

sudo -u vgvpnapi SSH_KEY=/home/vgvpnapi/.ssh/id_ed25519 \
	/opt/vg-dc-vpnapi/delegation-sync.sh || \
	echo "WARNING: delegation-sync.sh returned non-zero — DNS may not be updated"

END_TIME="$(date +%s)"

echo

# ---------------------------------------------------------------------------
# STATS
# ---------------------------------------------------------------------------

ELAPSED_SWITCH=$(( END_TIME_SWITCH - START_TIME ))
ELAPSED_TOTAL=$(( END_TIME - START_TIME ))
ELAPSED_FMT="$(printf '%02d:%02d:%02d' $((ELAPSED_TOTAL/3600)) $(( (ELAPSED_TOTAL%3600)/60 )) $((ELAPSED_TOTAL%60)))"

SWITCHED="$(grep -c "^Brigade:" "${SWITCH_LOG}" 2>/dev/null || echo 0)"
SWITCH_OK="$(grep -c "Success" "${SWITCH_LOG}" 2>/dev/null || echo 0)"

echo "=== SWITCH STATS ==="
echo "  Brigades processed:  ${SWITCHED}"
echo "  Confirmed switched:  ${SWITCH_OK}"
echo "  Elapsed:             ${ELAPSED_FMT}"
echo "  Log:                 ${SWITCH_LOG}"
echo
echo "DNS is now pointing to new instances."
echo "Wait ~24h for propagation, then run:"
echo "  ./03-migr-defragnet-massive-cleanup.sh -tag ${TAG}"
