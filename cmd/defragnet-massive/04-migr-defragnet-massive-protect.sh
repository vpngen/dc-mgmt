#!/bin/sh
#
# Set protected_until on migrated brigades so getwasted cannot reap them.
#
# Run AFTER 02-migr-defragnet-massive-swith.sh, once the new instances are main.
#
# Why this is needed:
#   getwasted inactive deletes a brigade when ALL of these hold
#   (see cmd/getwasted/main.go, sqlGetInactive):
#       - active_users_count < MINACTIVE (5)
#       - protected_until IS NULL OR <= now() AT TIME ZONE 'UTC'
#       - b.main = true
#       - no reservation on the endpoint
#       - NO non-main instance exists            <-- b2.brigade_id IS NULL
#       - and, for a migrated brigade, instance_created_at older than 8 days
#
#   A block-driven migration leaves the old instance parked, so the brigade is
#   exempt via the "no non-main instance" clause -- but ONLY until that parked
#   instance is destroyed. The day cleanup runs, a freshly migrated brigade with
#   a low active count becomes deletable. protected_until is the only guard that
#   survives cleanup.
#
# Idempotent: uses GREATEST, so it only ever raises the value. Safe to re-run,
# and safe to run on a tag that was already protected.

set -e

DB_URL=${DB_URL:-"postgres:///vgrealm"}
PROTECT_DAYS=${PROTECT_DAYS:-14}

print_usage() {
	echo "Usage: $0 -tag <tag> [-days N] [-n]"
	echo "   or: $0 -f <prepared-file> [-days N] [-n]"
	echo
	echo "  -tag   the TAG used by 01-migr-defragnet-massive-propagade.sh"
	echo "  -f     explicit prepared file (overrides -tag)"
	echo "  -days  protection window in days (default: ${PROTECT_DAYS})"
	echo "  -n     dry run - show what would change, change nothing"
	exit 1
}

TAG=""
PREPARED_FILE=""
DRY_RUN=""

while [ $# -gt 0 ]; do
	case "$1" in
		-tag)  TAG="$2";           shift 2 ;;
		-f)    PREPARED_FILE="$2"; shift 2 ;;
		-days) PROTECT_DAYS="$2";  shift 2 ;;
		-n)    DRY_RUN=yes;        shift ;;
		-h|--help) print_usage ;;
		*) echo "Unknown option: $1"; print_usage ;;
	esac
done

if [ -z "${TAG}" ] && [ -z "${PREPARED_FILE}" ]; then
	echo "ERROR: -tag or -f is required"
	print_usage
fi

if ! echo "${PROTECT_DAYS}" | grep -Eq '^[0-9]+$'; then
	echo "ERROR: -days must be a positive integer: ${PROTECT_DAYS}"
	exit 1
fi

if [ -z "${PREPARED_FILE}" ]; then
	PREPARED_FILE="$(ls "${HOME}/tmp/${TAG}-migr-prepared-"*.json 2>/dev/null | tail -n 1)"
fi

if [ -z "${PREPARED_FILE}" ] || [ ! -s "${PREPARED_FILE}" ]; then
	echo "ERROR: no prepared file found for tag '${TAG}' in ${HOME}/tmp/"
	exit 1
fi

echo "PREPARED_FILE: ${PREPARED_FILE}"
echo "PROTECT_DAYS:  ${PROTECT_DAYS}"

# base32 brigade ids -> comma separated uuids (same conversion switch_local_migr.sh uses)
IDS=""
COUNT=0
for b32 in $(jq -r '.snaps[].brigade_id' < "${PREPARED_FILE}"); do
	hex="$(echo "${b32}=========" | base32 -d 2>/dev/null | hexdump -ve '1/1 "%02x"')"
	if [ ${#hex} -ne 32 ]; then
		echo "  [SKIP] cannot decode brigade id: ${b32}"
		continue
	fi
	uuid="$(echo "${hex}" | sed -E 's/^(.{8})(.{4})(.{4})(.{4})(.{12})$/\1-\2-\3-\4-\5/')"
	if [ -z "${IDS}" ]; then IDS="${uuid}"; else IDS="${IDS},${uuid}"; fi
	COUNT=$((COUNT + 1))
done

if [ "${COUNT}" -eq 0 ]; then
	echo "ERROR: no brigade ids decoded from ${PREPARED_FILE}"
	exit 1
fi

echo "BRIGADES:      ${COUNT}"
echo

if [ -n "${DRY_RUN}" ]; then
	echo ">>> DRY RUN - showing current state, changing nothing"
	psql "${DB_URL}" -v ids="${IDS}" -v days="${PROTECT_DAYS}" <<'EOSQL'
SELECT b.domain_name,
       host(b.endpoint_ipv4)                AS ip,
       s.active_users_count                 AS active,
       s.protected_until                    AS protected_now,
       GREATEST(s.protected_until,
                (now() AT TIME ZONE 'UTC')
                  + make_interval(days => :'days'::int)) AS protected_after
FROM stats.brigades_stats s
JOIN brigades.brigades b
  ON b.brigade_id = s.brigade_id AND b.instance_id = s.instance_id
WHERE b.main = true
  AND b.brigade_id = ANY(string_to_array(:'ids', ',')::uuid[])
ORDER BY b.domain_name;
EOSQL
	exit 0
fi

echo ">>> SETTING protected_until = now + ${PROTECT_DAYS} days (GREATEST, never lowers)"
# Needs sql/patches/039-stats-migr-carryforward-grant.sql applied for vgmigr.
psql "${DB_URL}" -v ids="${IDS}" -v days="${PROTECT_DAYS}" <<'EOSQL'
UPDATE stats.brigades_stats s
SET protected_until = GREATEST(s.protected_until,
                               (now() AT TIME ZONE 'UTC')
                                 + make_interval(days => :'days'::int))
FROM brigades.brigades b
WHERE b.brigade_id  = s.brigade_id
  AND b.instance_id = s.instance_id
  AND b.main = true
  AND b.brigade_id = ANY(string_to_array(:'ids', ',')::uuid[]);
EOSQL

echo
echo ">>> RESULT"
psql "${DB_URL}" -v ids="${IDS}" <<'EOSQL'
SELECT count(*)                                                          AS brigades,
       count(*) FILTER (WHERE s.protected_until
                              > (now() AT TIME ZONE 'UTC'))              AS protected_ok,
       count(*) FILTER (WHERE s.protected_until IS NULL
                              OR s.protected_until
                                 <= (now() AT TIME ZONE 'UTC'))          AS unprotected_BAD,
       min(s.protected_until)                                            AS earliest_expiry
FROM stats.brigades_stats s
JOIN brigades.brigades b
  ON b.brigade_id = s.brigade_id AND b.instance_id = s.instance_id
WHERE b.main = true
  AND b.brigade_id = ANY(string_to_array(:'ids', ',')::uuid[]);
EOSQL

echo
echo "PROTECT COMPLETE: ${PREPARED_FILE}"
