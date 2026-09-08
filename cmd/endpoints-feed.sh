#!/bin/sh
# Export the endpoint IPv4 list as CSV and POST it to the external monitoring feed.
#
# Columns:
#   endpoint_ipv4 - the address itself
#   enabled       - address is in the reservation pool, i.e. available for new brigades
#   has_brigade   - some brigade occupies this address
#   is_main       - that brigade is the live one; false means a migrated leftover
#                   that only keeps the ports busy
#
# The remote list is replaced wholesale on every POST, so always send the full
# export, never a delta. Re-sending an identical file is a no-op on their side.

set -e

CONFDIR=${CONFDIR:-"/etc/vg-dc-stats"}

if [ -r "${CONFDIR}/endpoints-feed.env" ]; then
        # shellcheck source=/dev/null
        . "${CONFDIR}/endpoints-feed.env"
fi

DBNAME=${DBNAME:-"vgrealm"}

# DRY_RUN=1 writes the CSV to stdout and sends nothing. Credentials are only
# required for a real send, so the export can be reviewed before the token exists.
DRY_RUN=${DRY_RUN:-""}

if [ -z "${DRY_RUN}" ]; then
        API_URL=${API_URL:?"not set, see ${CONFDIR}/endpoints-feed.env-sample"}
        API_TOKEN=${API_TOKEN:?"not set, see ${CONFDIR}/endpoints-feed.env-sample"}
fi

# Since the feed is a full replace, a truncated export would silently wipe the
# monitored set. Refuse to send anything suspiciously short.
MIN_ROWS=${MIN_ROWS:-"1000"}

OUTFILE="$(mktemp "${TMPDIR:-/tmp}/endpoints-feed.XXXXXXXX")"
trap 'rm -f "${OUTFILE}"' EXIT INT TERM

psql -d "${DBNAME}" -q -v ON_ERROR_STOP=1 > "${OUTFILE}" <<'SQL'
COPY (
        SELECT
                pe.endpoint_ipv4,
                pe.enabled,
                EXISTS (
                        SELECT 1 FROM brigades.brigades b
                        WHERE b.endpoint_ipv4 = pe.endpoint_ipv4
                ) AS has_brigade,
                EXISTS (
                        SELECT 1 FROM brigades.brigades b
                        WHERE b.endpoint_ipv4 = pe.endpoint_ipv4 AND b.main
                ) AS is_main
        FROM pairs.pairs_endpoints_ipv4 pe
        -- Primary endpoints only, matching what brigades.active_pairs counts as
        -- a slot. Every row has endpoint_num = 0 today; the filter keeps the feed
        -- from silently changing shape if secondary endpoints ever appear.
        WHERE pe.endpoint_num = 0
        ORDER BY pe.endpoint_ipv4
) TO STDOUT WITH (FORMAT csv, HEADER)
SQL

rows="$(($(wc -l < "${OUTFILE}") - 1))"

echo "[i] Exported ${rows} endpoints" >&2

if [ -n "${DRY_RUN}" ]; then
        echo "[i] Dry run, nothing sent" >&2
        cat "${OUTFILE}"

        exit 0
fi

if [ "${rows}" -lt "${MIN_ROWS}" ]; then
        echo "[-] Refusing to send: ${rows} rows, below MIN_ROWS=${MIN_ROWS}" >&2

        exit 1
fi

echo "[i] POST ${API_URL}" >&2

curl -sS -f \
        -X POST \
        -H "Authorization: Bearer ${API_TOKEN}" \
        -H "Content-Type: text/csv" \
        --data-binary "@${OUTFILE}" \
        "${API_URL}"

echo
echo "[i] Sent ${rows} endpoints" >&2
