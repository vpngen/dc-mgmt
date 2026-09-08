#!/bin/sh
# Manual variant of endpoints-feed.sh: writes the CSV to a file and sends nothing.
# Deliberately not packaged — scp it to the head node and run it there when you
# want to look at the export without touching the monitoring feed.
#
# Usage:
#   sh endpoints-list.sh                     -> /tmp/20260824_143012_ip_list.csv
#   OUTDIR=/home/vgstats sh endpoints-list.sh
#
# Columns:
#   endpoint_ipv4 - the address itself
#   enabled       - address is in the reservation pool, i.e. available for new brigades
#   has_brigade   - some brigade occupies this address
#   is_main       - that brigade is the live one; false means a migrated leftover
#                   that only keeps the ports busy

set -e

DBNAME=${DBNAME:-"vgrealm"}
OUTDIR=${OUTDIR:-"/tmp"}
OUTFILE="${OUTDIR}/$(date +%Y%m%d_%H%M%S)_ip_list.csv"

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
        -- Primary endpoints only, matching what brigades.active_pairs counts as a slot.
        WHERE pe.endpoint_num = 0
        ORDER BY pe.endpoint_ipv4
) TO STDOUT WITH (FORMAT csv, HEADER)
SQL

rows="$(($(wc -l < "${OUTFILE}") - 1))"

echo "[i] Wrote ${OUTFILE} (${rows} endpoints)"
