#!/bin/sh
#
# Record today's reachability verdict for every endpoint address.
#
# Run daily, AFTER endpoints-feed.sh has published the current endpoint list --
# the monitor judges whatever list it was last given, so ingesting before the
# feed records verdicts about yesterday's fleet.
#
# The monitor returns only PROBLEM addresses. Anything in our DB that is absent
# from that list is reachable, and is recorded as 'ok'. That derived row is the
# whole point: without it there is no way to ask "clean for three days running",
# and every rotation decision is about a streak rather than a reading.
#
# One row per address per day; re-running overwrites today's row, so the job is
# idempotent and safe to retry.

set -e

CONFDIR=${CONFDIR:-"/etc/vg-dc-stats"}

if [ -r "${CONFDIR}/endpoints-feed.env" ]; then
        # shellcheck source=/dev/null
        . "${CONFDIR}/endpoints-feed.env"
fi

DBNAME=${DBNAME:-"vgrealm"}
STATS_SCHEMA=${STATS_SCHEMA:-"stats"}

# Defaults to the status endpoint alongside the feed URL we already publish to.
STATUS_URL=${STATUS_URL:-"${API_URL}/status?select=problems&nodes=1"}

# DRY_RUN=1 fetches and summarises, writes nothing to the database.
DRY_RUN=${DRY_RUN:-""}

# Our own liveness probe. blocked_ru with a dead port 80 is not a blocking
# problem -- the endpoint is down -- and must not enter a migration batch.
PROBE=${PROBE:-"1"}
NC_TIMEOUT=${NC_TIMEOUT:-"3"}
NC_PARALLEL=${NC_PARALLEL:-"50"}

# The monitor's snapshot has its own age, separate from our feed's. Acting on a
# stale one means acting on a fleet that has since been migrated.
MAX_SNAPSHOT_AGE_H=${MAX_SNAPSHOT_AGE_H:-"36"}

# generated_at is when the response was produced, not when the probing ran, so
# it is always ~now and proves nothing. The real freshness signal is the
# per-node age_seconds. The monitor needs ~12h to work through a freshly
# published list, and this job is timed 14h after the feed -- if the median
# probe is older than this, the checks have not caught up and today's verdicts
# describe an earlier fleet.
MAX_PROBE_AGE_H=${MAX_PROBE_AGE_H:-"24"}

# A truncated or empty response would mark the whole fleet 'ok' and make every
# parked probe look reclaimable. Refuse rather than record that.
MIN_PROBLEMS=${MIN_PROBLEMS:-"100"}

if [ -z "${API_TOKEN}" ]; then
        echo "[-] API_TOKEN not set, see ${CONFDIR}/endpoints-feed.env-sample" >&2
        exit 1
fi

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/reachability.XXXXXXXX")"
trap 'rm -rf "${WORKDIR}"' EXIT INT TERM

JSON="${WORKDIR}/status.json"
CSV="${WORKDIR}/verdicts.csv"
PROBES="${WORKDIR}/probes.csv"

echo "[i] GET ${STATUS_URL}" >&2
curl -sS -f -H "Authorization: Bearer ${API_TOKEN}" "${STATUS_URL}" > "${JSON}"

generated_at="$(jq -r '.generated_at // empty' < "${JSON}")"
if [ -z "${generated_at}" ]; then
        echo "[-] response has no generated_at; refusing" >&2
        exit 1
fi

age_h=$(( ( $(date -u +%s) - $(date -u -d "${generated_at}" +%s) ) / 3600 ))
echo "[i] snapshot generated_at=${generated_at} (${age_h}h old)" >&2
if [ "${age_h}" -gt "${MAX_SNAPSHOT_AGE_H}" ]; then
        echo "[-] snapshot is ${age_h}h old, limit ${MAX_SNAPSHOT_AGE_H}h; refusing" >&2
        exit 1
fi

jq -r '.counts | to_entries | map("\(.key)=\(.value)") | join(" ")' < "${JSON}" >&2

# Median probe age across every node reading in the response.
median_age_s="$(jq -r '[.addresses[].reach.nodes[]? | select(.age_seconds != null) | .age_seconds]
                       | sort
                       | if length == 0 then empty else .[length/2 | floor] end' < "${JSON}")"

if [ -z "${median_age_s}" ]; then
        echo "[-] no probe ages in response; cannot judge freshness, refusing" >&2
        exit 1
fi

median_age_h=$(( median_age_s / 3600 ))
echo "[i] median probe age ${median_age_h}h" >&2

if [ "${median_age_h}" -gt "${MAX_PROBE_AGE_H}" ]; then
        echo "[-] median probe age ${median_age_h}h exceeds ${MAX_PROBE_AGE_H}h --" >&2
        echo "    the monitor has not finished checking the current list; refusing" >&2
        exit 1
fi

jq -r '.addresses[]
       | [ .ip,
           .verdict,
           (.reference // ""),
           (.reach.ru_reachable   // .ru_reachable   // 0),
           (.reach.ru_unreachable // 0),
           (.reach.ru_no_data     // 0),
           (.reach.ru_total       // .ru_total       // 0) ]
       | @csv' < "${JSON}" > "${CSV}"

problems="$(wc -l < "${CSV}")"
echo "[i] ${problems} problem addresses" >&2

if [ "${problems}" -lt "${MIN_PROBLEMS}" ]; then
        echo "[-] only ${problems} problem rows, below MIN_PROBLEMS=${MIN_PROBLEMS}; refusing" >&2
        exit 1
fi

if [ -n "${DRY_RUN}" ]; then
        echo "[i] Dry run, nothing written" >&2
        jq -r '.addresses | group_by(.verdict)
               | map("\(.[0].verdict)\t\(length)") | .[]' < "${JSON}"
        exit 0
fi

psql -d "${DBNAME}" -q -v ON_ERROR_STOP=1 \
     -v stats_schema="${STATS_SCHEMA}" -v csv="${CSV}" <<'EOSQL'
CREATE TEMP TABLE ingest (
        ip              inet PRIMARY KEY,
        verdict         text,
        reference       text,
        ru_reachable    int,
        ru_unreachable  int,
        ru_no_data      int,
        ru_total        int
);

\copy ingest FROM :'csv' WITH (FORMAT csv)

-- Problem addresses: verbatim from the monitor.
INSERT INTO :"stats_schema".endpoint_reachability AS r
        (endpoint_ipv4, observed_on, verdict, reference,
         ru_reachable, ru_unreachable, ru_no_data, ru_total)
SELECT i.ip, (now() AT TIME ZONE 'UTC')::date, i.verdict, nullif(i.reference, ''),
       i.ru_reachable, i.ru_unreachable, i.ru_no_data, i.ru_total
FROM ingest i
ON CONFLICT (endpoint_ipv4, observed_on) DO UPDATE
SET verdict        = EXCLUDED.verdict,
    reference      = EXCLUDED.reference,
    ru_reachable   = EXCLUDED.ru_reachable,
    ru_unreachable = EXCLUDED.ru_unreachable,
    ru_no_data     = EXCLUDED.ru_no_data,
    ru_total       = EXCLUDED.ru_total,
    observed_at    = (now() AT TIME ZONE 'UTC');

-- Everything else we own is reachable. Recorded explicitly so a streak can be
-- counted; without these rows "absent" and "never observed" are the same thing.
INSERT INTO :"stats_schema".endpoint_reachability AS r
        (endpoint_ipv4, observed_on, verdict)
SELECT pei.endpoint_ipv4, (now() AT TIME ZONE 'UTC')::date, 'ok'
FROM pairs.pairs_endpoints_ipv4 pei
WHERE NOT EXISTS (SELECT 1 FROM ingest i WHERE i.ip = pei.endpoint_ipv4)
ON CONFLICT (endpoint_ipv4, observed_on) DO UPDATE
SET verdict     = EXCLUDED.verdict,
    observed_at = (now() AT TIME ZONE 'UTC');

\echo '--- recorded today ---'
SELECT verdict, count(*)
FROM :"stats_schema".endpoint_reachability
WHERE observed_on = (now() AT TIME ZONE 'UTC')::date
GROUP BY verdict ORDER BY 2 DESC;
EOSQL

if [ "${PROBE}" != "1" ]; then
        echo "[i] PROBE=0, skipping port 80 checks" >&2
        exit 0
fi

# Probe only what a decision depends on: a main that looks blocked. Everything
# recorded 'ok' was reached by the monitor, so its listener is alive by definition.
psql -d "${DBNAME}" -qtA -v stats_schema="${STATS_SCHEMA}" <<'EOSQL' > "${WORKDIR}/targets.txt"
SELECT host(r.endpoint_ipv4)
FROM :"stats_schema".endpoint_reachability r
JOIN brigades.brigades b ON b.endpoint_ipv4 = r.endpoint_ipv4 AND b.main
WHERE r.observed_on = (now() AT TIME ZONE 'UTC')::date
  AND r.verdict = 'blocked_ru';
EOSQL

targets="$(wc -l < "${WORKDIR}/targets.txt")"
echo "[i] probing port 80 on ${targets} blocked mains (${NC_PARALLEL} parallel)" >&2

if [ "${targets}" -gt 0 ]; then
        xargs -P "${NC_PARALLEL}" -I{} sh -c \
                'if nc -z -w '"${NC_TIMEOUT}"' "$1" 80 >/dev/null 2>&1; then echo "$1,t"; else echo "$1,f"; fi' \
                _ {} < "${WORKDIR}/targets.txt" > "${PROBES}"

        psql -d "${DBNAME}" -q -v ON_ERROR_STOP=1 \
             -v stats_schema="${STATS_SCHEMA}" -v csv="${PROBES}" <<'EOSQL'
CREATE TEMP TABLE probe (ip inet PRIMARY KEY, ok boolean);
\copy probe FROM :'csv' WITH (FORMAT csv)

UPDATE :"stats_schema".endpoint_reachability r
SET nc80_ok = p.ok
FROM probe p
WHERE r.endpoint_ipv4 = p.ip
  AND r.observed_on = (now() AT TIME ZONE 'UTC')::date;

\echo '--- blocked mains: port 80 alive? ---'
SELECT nc80_ok, count(*)
FROM :"stats_schema".endpoint_reachability
WHERE observed_on = (now() AT TIME ZONE 'UTC')::date
  AND verdict = 'blocked_ru'
GROUP BY 1 ORDER BY 1;
EOSQL
fi

echo "[i] done" >&2
