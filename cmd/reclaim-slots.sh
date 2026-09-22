#!/bin/sh
#
# Free slots held by parked instances whose address is no longer blocked.
#
# A parked (main = false) instance sits on an address RKN blocked, keeping
# ports 80/443 answering so the external monitor can tell us when the ban
# lifts. Once it has, the probe has done its job: destroy the instance, delete
# its rows, re-enable the endpoint, and the slot is reservable again.
#
# Reports by default and changes nothing. --apply is required to act.
#
# Usage:
#   reclaim-slots.sh                          report candidates, change nothing
#   reclaim-slots.sh --apply                  reclaim, serially, up to --limit
#   reclaim-slots.sh --apply --parallel 8     the same, 8 at a time
#   reclaim-slots.sh --ip 1.2.3.4             report just that address
#   reclaim-slots.sh --ip 1.2.3.4 --apply     reclaim just that address
#   reclaim-slots.sh --ip 1.2.3.4 --apply --force
#                                             skip the clean-days policy checks
#   reclaim-slots.sh --max-active 3           allow reclaim up to 3 active users
#
# --force relaxes POLICY (how long the address has looked clean, how old the
# instance is). It does NOT relax SAFETY: the instance must still be non-main,
# the brigade must still have a live main elsewhere, the address must not be
# reserved, and the instance must carry no more than --max-active users.
# Those four prevent deleting a live brigade, a migration in flight, or an
# instance people are still connecting through, and are never skipped.
#
# On that last one: 404 of 738 parked instances were found carrying live users
# (5,516 people, 2026-09-22). They connect with old configs to the parked
# endpoint, and are counted under the PARKED instance -- which MAU excludes,
# because MAU counts mains only. So destroying such an instance cuts off real
# users while the dashboard shows nothing at all. Default is 0: reclaim only
# what genuinely has nobody on it.

set -e

DBNAME=${DBNAME:-"vgrealm"}
STATS_SCHEMA=${STATS_SCHEMA:-"stats"}
SSH_TIMEOUT=${SSH_TIMEOUT:-"10"}

# ---------------------------------------------------------------------------
# Worker mode. The script re-invokes itself once per candidate so xargs -P can
# run several at a time; POSIX sh cannot export functions.
# ---------------------------------------------------------------------------
if [ "$1" = "--reclaim-one" ]; then
        line="$2"
        IFS='|' read -r bid iid ip ctrl okd verdict <<EOF
${line}
EOF
        b32="$(echo "${bid}" | tr -d '-' | xxd -r -p -l 16 | base32 | tr -d '=')"

        # Record intent before touching anything, so a crash leaves a trace.
        log_id="$(psql -d "${DBNAME}" -qtA -v ON_ERROR_STOP=1 \
                  -v stats_schema="${STATS_SCHEMA}" \
                  -v bid="${bid}" -v iid="${iid}" -v ip="${ip}" \
                  -v ctrl="${ctrl}" -v okd="${okd}" -v verdict="${verdict}" <<'EOSQL'
INSERT INTO :"stats_schema".slot_reclaim_log
        (endpoint_ipv4, brigade_id, instance_id, control_ip, clean_days, latest_verdict)
VALUES  ((:'ip')::inet, (:'bid')::uuid, (:'iid')::uuid, (:'ctrl')::inet,
         nullif(:'okd','')::int, nullif(:'verdict',''))
RETURNING id;
EOSQL
        )"

        # The base32 id is the same for every instance of a brigade, so the
        # control node is what selects which copy dies. The live main lives on
        # a different node and is untouched.
        if sudo -u vgvpnapi ssh -n -o BatchMode=yes -o StrictHostKeyChecking=accept-new \
                -o ConnectTimeout="${SSH_TIMEOUT}" "_serega_@${ctrl}" \
                destroy -force -id "${b32}" >/dev/null 2>&1; then
                :
        else
                psql -d "${DBNAME}" -q -v stats_schema="${STATS_SCHEMA}" -v id="${log_id}" <<'EOSQL'
UPDATE :"stats_schema".slot_reclaim_log
SET destroyed_ok = false, finished_at = (now() AT TIME ZONE 'UTC'),
    error = 'destroy failed on control node'
WHERE id = (:'id')::bigint;
EOSQL
                printf '%-16s %-28s DESTROY-FAILED\n' "${ip}" "${b32}" >&2
                exit 1
        fi

        if psql -d "${DBNAME}" -q -v ON_ERROR_STOP=1 -v stats_schema="${STATS_SCHEMA}" \
                -v iid="${iid}" -v ip="${ip}" -v id="${log_id}" <<'EOSQL'
BEGIN;
DELETE FROM brigades.reserved_endpoints_ipv4 WHERE endpoint_ipv4 = (:'ip')::inet;
DELETE FROM brigades.brigades                WHERE instance_id   = (:'iid')::uuid;
UPDATE pairs.pairs_endpoints_ipv4 SET enabled = true
 WHERE endpoint_ipv4 = (:'ip')::inet;
UPDATE :"stats_schema".slot_reclaim_log
SET destroyed_ok = true, db_deleted = true, endpoint_enabled = true,
    finished_at = (now() AT TIME ZONE 'UTC')
WHERE id = (:'id')::bigint;
COMMIT;
EOSQL
        then
                printf '%-16s %-28s OK\n' "${ip}" "${b32}" >&2
                exit 0
        fi

        psql -d "${DBNAME}" -q -v stats_schema="${STATS_SCHEMA}" -v id="${log_id}" <<'EOSQL'
UPDATE :"stats_schema".slot_reclaim_log
SET destroyed_ok = true, db_deleted = false, endpoint_enabled = false,
    finished_at = (now() AT TIME ZONE 'UTC'),
    error = 'instance destroyed but database update failed'
WHERE id = (:'id')::bigint;
EOSQL
        printf '%-16s %-28s DB-FAILED (instance already destroyed)\n' "${ip}" "${b32}" >&2
        exit 1
fi

# ---------------------------------------------------------------------------
# Normal mode
# ---------------------------------------------------------------------------
APPLY=""
FORCE=""
ONE_IP=""
LIMIT=${LIMIT:-"50"}
PARALLEL=${PARALLEL:-"1"}
MIN_CLEAN_DAYS=${MIN_CLEAN_DAYS:-"3"}

# A parked instance younger than this is not a probe -- it is the freshly
# restored target of a migration that has not switched yet.
MIN_INSTANCE_AGE_DAYS=${MIN_INSTANCE_AGE_DAYS:-"2"}

# Highest active_users_count a parked instance may carry and still be treated
# as abandoned. SAFETY, not policy: --force does not skip it.
MAX_ACTIVE_USERS=${MAX_ACTIVE_USERS:-"0"}

print_usage() {
        sed -n '3,30p' "$0"
        exit 1
}

while [ $# -gt 0 ]; do
        case "$1" in
                --apply)           APPLY=yes;            shift ;;
                --force)           FORCE=yes;            shift ;;
                --ip)              ONE_IP="$2";          shift 2 ;;
                --limit)           LIMIT="$2";           shift 2 ;;
                --parallel)        PARALLEL="$2";        shift 2 ;;
                --min-clean-days)  MIN_CLEAN_DAYS="$2";  shift 2 ;;
                --max-active)      MAX_ACTIVE_USERS="$2"; shift 2 ;;
                -h|--help)         print_usage ;;
                *) echo "Unknown option: $1" >&2; print_usage ;;
        esac
done

for n in "${LIMIT}" "${PARALLEL}" "${MIN_CLEAN_DAYS}" "${MIN_INSTANCE_AGE_DAYS}" "${MAX_ACTIVE_USERS}"; do
        echo "${n}" | grep -Eq '^[0-9]+$' || { echo "[-] not a number: ${n}" >&2; exit 1; }
done
[ "${PARALLEL}" -ge 1 ] || { echo "[-] --parallel must be >= 1" >&2; exit 1; }

if [ -n "${ONE_IP}" ]; then
        echo "${ONE_IP}" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' \
                || { echo "[-] --ip is not an IPv4 address: ${ONE_IP}" >&2; exit 1; }
fi

if [ -n "${FORCE}" ] && [ -z "${ONE_IP}" ]; then
        echo "[-] --force requires --ip: it skips the clean-days policy and is" >&2
        echo "    meant for one address you have judged yourself, not a batch" >&2
        exit 1
fi

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/reclaim.XXXXXXXX")"
trap 'rm -rf "${WORKDIR}"' EXIT INT TERM
CANDIDATES="${WORKDIR}/candidates.txt"

RUN_START="$(date -u +'%Y-%m-%d %H:%M:%S')"

# ---------------------------------------------------------------------------
# Candidate selection
#
# The three SAFETY clauses (non-main, brigade has a live main, not reserved)
# apply always, --force included:
#
#   NOT reserved -- a migration in flight has ALREADY created its new instance
#   with main = false on a clean target address, reserved until the switch.
#   Such a spare satisfies every other condition here. The reservation is the
#   only thing distinguishing it from a real probe.
#
#   brigade has a live main -- otherwise we delete a brigade's only instance.
#
#   no live users -- a parked instance carrying active_users_count > --max-active
#   is not an abandoned probe. Those users are invisible in MAU (mains only),
#   so destroying it harms people with nothing in the metric to show for it.
#   A NULL count is treated as unsafe: absence of a stats row is not evidence
#   of absence of users.
#
# The POLICY clauses (clean streak, instance age) are what --force skips.
# ---------------------------------------------------------------------------
psql -d "${DBNAME}" -qtA -F'|' -v ON_ERROR_STOP=1 \
     -v stats_schema="${STATS_SCHEMA}" \
     -v min_clean="${MIN_CLEAN_DAYS}" \
     -v min_age="${MIN_INSTANCE_AGE_DAYS}" \
     -v max_active="${MAX_ACTIVE_USERS}" \
     -v one_ip="${ONE_IP}" \
     -v force="${FORCE:-no}" <<'EOSQL' > "${CANDIDATES}"
SELECT b.brigade_id,
       b.instance_id,
       host(b.endpoint_ipv4),
       host(p.control_ip),
       coalesce(r.ok_days::text, ''),
       coalesce(r.latest_verdict, '')
FROM brigades.brigades b
JOIN pairs.pairs p ON p.pair_id = b.pair_id
LEFT JOIN :"stats_schema".endpoint_reachability_recent r
       ON r.endpoint_ipv4 = b.endpoint_ipv4
LEFT JOIN :"stats_schema".brigades_stats st
       ON st.brigade_id = b.brigade_id AND st.instance_id = b.instance_id
WHERE NOT b.main
  AND EXISTS (SELECT 1 FROM brigades.brigades m
               WHERE m.brigade_id = b.brigade_id AND m.main)
  AND NOT EXISTS (SELECT 1 FROM brigades.reserved_endpoints_ipv4 re
                   WHERE re.endpoint_ipv4 = b.endpoint_ipv4)
  -- SAFETY: people are still connecting through this instance with old
  -- configs. They are counted under the parked instance, which MAU excludes,
  -- so cutting them off is invisible in the metric. NULL means no stats row
  -- at all, which is not evidence of emptiness -- treat it as unsafe.
  AND st.active_users_count IS NOT NULL
  AND st.active_users_count <= (:'max_active')::int
  AND NOT (p.control_ip <<= '10.30.33.0/24')
  AND NOT (p.control_ip <<= '10.30.53.0/24')
  AND ((:'one_ip') = '' OR host(b.endpoint_ipv4) = (:'one_ip'))
  AND ( (:'force') = 'yes'
        OR ( r.days     >= (:'min_clean')::int
         AND r.ok_days   = r.days
         AND r.latest_verdict = 'ok'
         AND st.instance_created_at
             < (now() AT TIME ZONE 'UTC') - make_interval(days => (:'min_age')::int) ) )
ORDER BY r.ok_days DESC NULLS LAST, b.endpoint_ipv4;
EOSQL

found="$(wc -l < "${CANDIDATES}")"
echo "[i] ${found} slot(s) eligible${FORCE:+ (FORCED - policy checks skipped)}" >&2

if [ "${found}" -eq 0 ]; then
        echo "[i] nothing to do" >&2
        exit 0
fi

if [ -z "${APPLY}" ]; then
        echo
        echo "REPORT ONLY -- nothing changed. Re-run with --apply to reclaim."
        echo "${found} eligible; --apply would take the first ${LIMIT}."
        echo
        printf '%-16s %-16s %-9s %s\n' "ENDPOINT" "CONTROL" "CLEAN_D" "BRIGADE"
        while IFS='|' read -r bid iid ip ctrl okd verdict; do
                printf '%-16s %-16s %-9s %s\n' "${ip}" "${ctrl}" "${okd:--}" "${bid}"
        done < "${CANDIDATES}"
        exit 0
fi

head -n "${LIMIT}" "${CANDIDATES}" > "${CANDIDATES}.todo"
mv "${CANDIDATES}.todo" "${CANDIDATES}"
todo="$(wc -l < "${CANDIDATES}")"

echo "[i] APPLYING to ${todo} of ${found}, ${PARALLEL} at a time" >&2
echo >&2

# Workers report their own outcome; the run is summarised from the log table
# afterwards, which is authoritative and survives a lost terminal.
set +e
xargs -P "${PARALLEL}" -I{} "$0" --reclaim-one "{}" < "${CANDIDATES}"
set -e

echo >&2
psql -d "${DBNAME}" -q -v stats_schema="${STATS_SCHEMA}" -v since="${RUN_START}" <<'EOSQL'
SELECT count(*) FILTER (WHERE destroyed_ok AND db_deleted AND endpoint_enabled) AS reclaimed,
       count(*) FILTER (WHERE destroyed_ok IS NOT TRUE)                          AS destroy_failed,
       count(*) FILTER (WHERE destroyed_ok AND db_deleted IS NOT TRUE)           AS db_failed
FROM :"stats_schema".slot_reclaim_log
WHERE planned_at >= (:'since')::timestamp;
EOSQL

unfinished="$(psql -d "${DBNAME}" -qtA -v stats_schema="${STATS_SCHEMA}" -v since="${RUN_START}" <<'EOSQL'
SELECT count(*) FROM :"stats_schema".slot_reclaim_log
WHERE planned_at >= (:'since')::timestamp
  AND (destroyed_ok IS NOT TRUE OR db_deleted IS NOT TRUE OR endpoint_enabled IS NOT TRUE);
EOSQL
)"

if [ "${unfinished}" -gt 0 ]; then
        echo "[-] ${unfinished} need a human:" >&2
        echo "    SELECT * FROM ${STATS_SCHEMA}.slot_reclaim_unfinished;" >&2
        exit 1
fi
