#!/bin/sh

DB_URL=${DB_URL:-"postgres:///vgrealm"}

set -e

if [ $# -eq 0 ]; then
	echo "Usage: $0 <network1> [network2 ...]"
	echo "  Disables endpoint IPs in the given networks so no new brigades are placed there."
	exit 1
fi

for NETWORK in "$@"; do
	echo "PREPARE NETWORK: ${NETWORK}"

	psql "${DB_URL}" -v net="${NETWORK}" <<'EOSQL'
BEGIN;

UPDATE
	pairs.pairs_endpoints_ipv4
SET
	enabled=false
WHERE
	endpoint_ipv4<<=:'net';

SELECT
	p.control_ip
FROM
	pairs.pairs_endpoints_ipv4 i
JOIN
	pairs.pairs p ON i.pair_id=p.pair_id
WHERE
	i.endpoint_ipv4 <<= :'net'
GROUP BY
	p.control_ip;

COMMIT;
EOSQL

	echo "DONE: ${NETWORK}"
	echo
done
