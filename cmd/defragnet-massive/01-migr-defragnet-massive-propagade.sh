#!/bin/sh
#
# Massive brigade migration: snapshot → reserve → recode → restore.
# Leaves new (spare) instances on target servers; brigades still serve from old servers.
# Next step: 02-migr-defragnet-massive-swith.sh -tag <TAG>

if [ -s "${HOME}/.secret/local_migration.env" ]; then
	# shellcheck source=/dev/null
	. "${HOME}/.secret/local_migration.env"
fi

set -e

mkdir -p "${HOME}/migr-logs"
mkdir -p "${HOME}/tmp"

if [ -z "${AUTHFP}" ]; then
	echo "ERROR: AUTHFP not set"
	exit 1
fi

if [ -z "${REALM_FP}" ]; then
	echo "ERROR: REALM_FP not set"
	exit 1
fi

# ---------------------------------------------------------------------------
# arg parsing
# ---------------------------------------------------------------------------

print_usage() {
	echo "Usage: $0 -in <target-ctrl-cidr> [options]"
	echo
	echo "Required:"
	echo "  -in   <cidr>       target control network CIDR (where to reserve new slots)"
	echo
	echo "Optional:"
	echo "  -net  <cidr>       source endpoint CIDR filter (default: 0.0.0.0/0 = all endpoints)"
	echo "  -ctrl <cidr,...>   source control network(s), comma-separated (default: 0.0.0.0/0 = all)"
	echo "  -en   <cidr>       target endpoint network filter, e.g. VIP subnet"
	echo "  -count <N>         max brigades to migrate in one run (default: all from snapshot)"
	echo "  -tag  <tag>        custom base tag for file naming (default: auto-derived from CIDRs)"
	echo
	echo "Common usage — migrate by control network:"
	echo "  $0 -ctrl 10.30.1.0/24,10.30.2.0/24 -in 10.40.0.0/16"
	echo
	echo "Common usage — migrate by endpoint IP range to VIP servers:"
	echo "  $0 -net 4.33.222.0/24 -in 10.40.0.0/16 -en 5.44.111.0/24"
	echo
	echo "Env overrides:"
	echo "  MNT_TO=<unix-ts>   maintenance mode end time (default: now+7d)"
	echo "  SNAPSHOT=<path>    skip collection, use this snapshot file"
	echo "  FORCE_ERRORS=1     continue even if snapshot has errors_count > 0"
	exit 1
}

NET=""
CTRL=""
TARGET_IN=""
TARGET_EN=""
MAX_COUNT=""
CUSTOM_TAG=""

while [ $# -gt 0 ]; do
	case "$1" in
		-net)   NET="$2";        shift 2 ;;
		-ctrl)  CTRL="$2";       shift 2 ;;
		-in)    TARGET_IN="$2";  shift 2 ;;
		-en)    TARGET_EN="$2";  shift 2 ;;
		-count) MAX_COUNT="$2";  shift 2 ;;
		-tag)   CUSTOM_TAG="$2"; shift 2 ;;
		-h|--help) print_usage ;;
		*) echo "Unknown option: $1"; print_usage ;;
	esac
done

if [ -z "${TARGET_IN}" ]; then
	echo "ERROR: -in is required"
	print_usage
fi

if [ -z "${NET}" ]; then
	NET="0.0.0.0/0"
fi

if [ -z "${CTRL}" ]; then
	CTRL="0.0.0.0/0"
fi

# ---------------------------------------------------------------------------
# derive TAG for file naming
# ---------------------------------------------------------------------------

NET_BASE="${NET%%/*}"
IN_BASE="${TARGET_IN%%/*}"
CTRL_BASE="$(echo "${CTRL}" | cut -d',' -f1 | cut -d'/' -f1)"

# When -net is not specified (0.0.0.0/0), the ctrl range is the meaningful identifier.
# Tag format: ctrl-based when net is default, net+ctrl when both are explicit.
if [ "${NET}" = "0.0.0.0/0" ]; then
	TAG="${CUSTOM_TAG:-"${CTRL_BASE}-to-${IN_BASE}-massive"}"
else
	TAG="${CUSTOM_TAG:-"${NET_BASE}-${CTRL_BASE}-to-${IN_BASE}-massive"}"
fi

MNT_TO="${MNT_TO:-"$(date -d '+7 day' +%s)"}"

echo "=== MASSIVE MIGRATION: PROPAGADE ==="
echo "NET=${NET}  CTRL=${CTRL}"
echo "TARGET_IN=${TARGET_IN}  TARGET_EN=${TARGET_EN:-"(any)"}"
echo "TAG=${TAG}  MAX_COUNT=${MAX_COUNT:-"all"}"
echo

# ---------------------------------------------------------------------------
# STEP 1: snapshot
# ---------------------------------------------------------------------------

echo ">>> STEP 1: SNAPSHOT"

if [ -z "${SNAPSHOT}" ]; then
	echo "sudo -u vgsnaps /opt/vg-dc-snaps/collectsnaps.sh -tag ${TAG}-migr -net ${NET} -ctrl ${CTRL} -ad -mnt ${MNT_TO}"
	sudo -u vgsnaps /opt/vg-dc-snaps/collectsnaps.sh \
		-tag "${TAG}-migr" \
		-net "${NET}" \
		-ctrl "${CTRL}" \
		-ad \
		-mnt "${MNT_TO}"
fi

SNAPSHOT="${SNAPSHOT:-"$(ls "/vg-snapshots/${TAG}-migr/${TAG}"*.json 2>/dev/null | tail -n 1)"}"
if [ -z "${SNAPSHOT}" ] || [ ! -s "${SNAPSHOT}" ]; then
	echo "ERROR: no snapshot produced in /vg-snapshots/${TAG}-migr/"
	exit 1
fi

jq -r '"  errors_count=\(.errors_count), total_count=\(.total_count)"' < "${SNAPSHOT}"
echo "  SNAPSHOT=${SNAPSHOT}"

ERRORS_COUNT="$(jq -r '.errors_count' < "${SNAPSHOT}")"
if [ "${ERRORS_COUNT}" -gt 0 ]; then
	if [ -z "${FORCE_ERRORS}" ]; then
		echo "ERROR: snapshot has ${ERRORS_COUNT} error(s) — set FORCE_ERRORS=1 to proceed anyway"
		exit 1
	fi
	echo "WARNING: ${ERRORS_COUNT} snapshot error(s), continuing because FORCE_ERRORS is set"
fi

DTMARK="$(basename "${SNAPSHOT}" | sed 's/^.*\-migr\-//; s/\.json$//')"
SNAP_TOTAL="$(jq -r '.total_count' < "${SNAPSHOT}")"

echo

# ---------------------------------------------------------------------------
# STEP 2: apply COUNT cap (trim snapshot if needed)
# ---------------------------------------------------------------------------

echo ">>> STEP 2: COUNT"

if [ -n "${MAX_COUNT}" ] && [ "${MAX_COUNT}" -lt "${SNAP_TOTAL}" ]; then
	COUNT="${MAX_COUNT}"
	TRIMMED_SNAPSHOT="${HOME}/tmp/${TAG}-migr-trimmed-${DTMARK}.json"
	echo "  Trimming snapshot from ${SNAP_TOTAL} to ${COUNT} brigades"
	jq ".snaps = .snaps[:${COUNT}] | .total_count = ${COUNT} | .errors_count = 0" \
		"${SNAPSHOT}" > "${TRIMMED_SNAPSHOT}"
	SNAPSHOT="${TRIMMED_SNAPSHOT}"
else
	COUNT="${SNAP_TOTAL}"
fi

echo "  Migrating ${COUNT} brigade(s)"
echo "  DTMARK=${DTMARK}"
echo

# ---------------------------------------------------------------------------
# STEP 3: reservation (free slot check is done inside create_reservation.sh)
# ---------------------------------------------------------------------------

echo ">>> STEP 3: RESERVATION"

RESERV_TMP="${HOME}/tmp/.reserv-massive-$$.txt"

if [ -n "${TARGET_EN}" ]; then
	echo "  /opt/vg-dc-snaps/create_reservation.sh create -nc -in ${TARGET_IN} -en ${TARGET_EN} ${COUNT}"
	/opt/vg-dc-snaps/create_reservation.sh create -nc \
		-in "${TARGET_IN}" \
		-en "${TARGET_EN}" \
		"${COUNT}" 2>&1 | tee "${RESERV_TMP}" || true
else
	echo "  /opt/vg-dc-snaps/create_reservation.sh create -nc -in ${TARGET_IN} ${COUNT}"
	/opt/vg-dc-snaps/create_reservation.sh create -nc \
		-in "${TARGET_IN}" \
		"${COUNT}" 2>&1 | tee "${RESERV_TMP}" || true
fi

RESERVATION="$(grep "Created reservation:" "${RESERV_TMP}" | cut -f 3 -d ' ')"
rm -f "${RESERV_TMP}"

if ! echo "${RESERVATION}" | grep -Eq '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'; then
	echo "ERROR: reservation failed or returned invalid UUID"
	exit 1
fi

RSHORT="${RESERVATION%%-*}"
echo "  RESERVATION=${RESERVATION} (${RSHORT})"

RESERVATION_CONFIG_FILE="${HOME}/tmp/${TAG}-migr-reserv-${RSHORT}.json"
/opt/vg-dc-snaps/create_reservation.sh genconf "${RESERVATION}" > "${RESERVATION_CONFIG_FILE}"
echo "  RESERVATION_CONFIG_FILE=${RESERVATION_CONFIG_FILE}"
echo

# ---------------------------------------------------------------------------
# STEP 4: server availability check
# ---------------------------------------------------------------------------

echo ">>> STEP 4: AVAILABILITY CHECK"

if ! command -v nc >/dev/null 2>&1; then
	echo "  WARNING: nc not found, skipping availability check"
else
	AVAIL_OK=0
	AVAIL_FAIL=0
	AVAIL_FAIL_LIST=""

	for CTRL_IP in $(jq -r '.plan[].control_ip' "${RESERVATION_CONFIG_FILE}"); do
		if nc -z -w 5 "${CTRL_IP}" 22 2>/dev/null; then
			echo "  OK    ${CTRL_IP}:22"
			AVAIL_OK=$((AVAIL_OK + 1))
		else
			echo "  FAIL  ${CTRL_IP}:22 — unreachable!"
			AVAIL_FAIL=$((AVAIL_FAIL + 1))
			AVAIL_FAIL_LIST="${AVAIL_FAIL_LIST} ${CTRL_IP}"
		fi
	done

	echo "  Reachable: ${AVAIL_OK}  Unreachable: ${AVAIL_FAIL}"

	if [ "${AVAIL_FAIL}" -gt 0 ]; then
		echo "ERROR: ${AVAIL_FAIL} target server(s) unreachable:${AVAIL_FAIL_LIST}"
		echo "       Delete reservation and fix before retrying:"
		echo "       /opt/vg-dc-snaps/create_reservation.sh delete -f ${RESERVATION}"
		exit 1
	fi
fi

echo

# ---------------------------------------------------------------------------
# STEP 5: snap_prepare (decrypt with realm key, re-encrypt for authority key)
# ---------------------------------------------------------------------------

echo ">>> STEP 5: SNAP PREPARE"

PREPARED_FILE="${HOME}/tmp/${TAG}-migr-prepared-${DTMARK}.json"
echo "  /opt/vg-dc-snaps/snap_prepare -fp ${AUTHFP} < ${SNAPSHOT} > ${PREPARED_FILE}"
/opt/vg-dc-snaps/snap_prepare -fp "${AUTHFP}" < "${SNAPSHOT}" > "${PREPARED_FILE}"
echo "  PREPARED_FILE=${PREPARED_FILE}"
echo

# ---------------------------------------------------------------------------
# STEP 6: recodesnaps (assign reservation slots, re-encrypt for target realm)
# ---------------------------------------------------------------------------

echo ">>> STEP 6: RECODE"

PLAN_FILE="${HOME}/tmp/${TAG}-migr-plan-${DTMARK}.json"
echo "  /opt/vg-dc-snaps/recodesnaps -tfp ${REALM_FP} -c ... -in ... -out ${PLAN_FILE}"
/opt/vg-dc-snaps/recodesnaps \
	-tfp "${REALM_FP}" \
	-c "${RESERVATION_CONFIG_FILE}" \
	-rkeys "/etc/vg-dc-snaps/realms_keys" \
	-in "${PREPARED_FILE}" \
	-out "${PLAN_FILE}"
echo "  PLAN_FILE=${PLAN_FILE}"
echo

# ---------------------------------------------------------------------------
# STEP 7: restore (SSH to target routers, install spare brigade instances)
# ---------------------------------------------------------------------------

echo ">>> STEP 7: RESTORE"

LOG_FILE="${HOME}/migr-logs/$(date +"%Y%m%d%H%M")-${TAG}-migr-${DTMARK}.log"
echo "  LOG=${LOG_FILE}"

START_TIME="$(date +%s)"
/opt/vg-dc-snaps/restoresnaps -r "${RESERVATION}" -f "${PLAN_FILE}" 2>&1 | tee "${LOG_FILE}"
END_TIME="$(date +%s)"

echo

# ---------------------------------------------------------------------------
# STATS
# restoresnaps errors are at the router-group level: when a router fails,
# the entire group of brigades assigned to it is skipped (continue CTRL).
# We correlate ERROR log lines → router IPs → brigade IDs from the plan file.
# ---------------------------------------------------------------------------

ELAPSED=$(( END_TIME - START_TIME ))
ELAPSED_FMT="$(printf '%02d:%02d:%02d' $((ELAPSED/3600)) $(( (ELAPSED%3600)/60 )) $((ELAPSED%60)))"

PLAN_TOTAL="$(jq '[.plan[].snaps | length] | add // 0' "${PLAN_FILE}")"

# unique router IPs that had an ERROR line in the log
FAILED_ROUTER_IPS="$(grep "^ERROR: control_ip:" "${LOG_FILE}" 2>/dev/null \
	| sed 's/ERROR: control_ip: //; s/ .*//' | sort -u || true)"

# count brigades on failed routers and routers themselves
FAILED_BRIGADES=0
FAILED_ROUTERS=0
for _ip in ${FAILED_ROUTER_IPS}; do
	FAILED_ROUTERS=$((FAILED_ROUTERS + 1))
	_n="$(jq --arg ip "${_ip}" \
		'[.plan[] | select(.control_ip == $ip) | .snaps | length] | add // 0' \
		"${PLAN_FILE}")"
	FAILED_BRIGADES=$((FAILED_BRIGADES + _n))
done

SUCCESS_BRIGADES=$((PLAN_TOTAL - FAILED_BRIGADES))

echo "=== RESTORE STATS ==="
echo "  Brigades in plan:    ${PLAN_TOTAL}"
echo "  Migrated OK:         ${SUCCESS_BRIGADES}"
echo "  Failed:              ${FAILED_BRIGADES}  (across ${FAILED_ROUTERS} router(s))"
echo "  Elapsed:             ${ELAPSED_FMT}"
echo "  Log:                 ${LOG_FILE}"

if [ "${FAILED_BRIGADES}" -gt 0 ]; then
	echo
	echo "  Failed brigades (brigade_id) by router:"
	for _ip in ${FAILED_ROUTER_IPS}; do
		_n="$(jq --arg ip "${_ip}" \
			'[.plan[] | select(.control_ip == $ip) | .snaps | length] | add // 0' \
			"${PLAN_FILE}")"
		echo "  Router ${_ip} (${_n} brigade(s)):"
		jq -r --arg ip "${_ip}" \
			'.plan[] | select(.control_ip == $ip) | .snaps[].brigade_id' \
			"${PLAN_FILE}" | sed 's/^/    /' || true
		echo "  Error:"
		grep "^ERROR: control_ip: ${_ip}" "${LOG_FILE}" | sed 's/^/    /' || true
		echo
	done
fi

echo
echo "=== SUMMARY ==="
echo "  TAG=${TAG}"
echo "  RESERVATION=${RESERVATION}"
echo "  SNAPSHOT=${SNAPSHOT}"
echo "  PLAN_FILE=${PLAN_FILE}"
echo "  PREPARED_FILE=${PREPARED_FILE}"
echo
echo "Next steps:"
echo "  1. Run: ./02-migr-defragnet-massive-swith.sh -tag ${TAG}"
echo "     (switches DNS to new instances + runs delegation-sync)"
echo "  2. Wait ~24h for DNS propagation"
echo "  3. Run: ./03-migr-defragnet-massive-cleanup.sh -tag ${TAG}"
echo "     (destroys old brigade instances + releases reservation)"
