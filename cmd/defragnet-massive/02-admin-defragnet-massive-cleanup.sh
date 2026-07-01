#!/bin/sh
#
# Massive brigade migration: remove source endpoint IPs from the pairs DB.
# Run this after 03-migr-defragnet-massive-cleanup.sh completes.

DB_URL=${DB_URL:-"postgres:///vgrealm"}

set -e

if [ $# -eq 0 ]; then
	echo "Usage: $0 <network1> [network2 ...]"
	echo "  Removes endpoint IPs and empty pair records for each given source network."
	exit 1
fi

for NETWORK in "$@"; do
	echo "CLEANUP NETWORK: ${NETWORK}"

	psql "${DB_URL}" -v net="${NETWORK}" <<'EOSQL'
BEGIN;

DELETE FROM
	brigades.orphaned_endpoints_ipv4
WHERE
	endpoint_ipv4<<=:'net';

DELETE FROM
	brigades.domains_endpoints_ipv4
WHERE
	endpoint_ipv4<<=:'net';

DELETE FROM
	pairs.pairs_endpoints_ipv4
WHERE
	endpoint_ipv4<<=:'net';

DELETE FROM
	pairs.pairs
WHERE
	pair_id IN (
		SELECT
			p.pair_id
		FROM
			pairs.pairs p
		WHERE
			(SELECT
				count(*)
			FROM
				pairs.pairs_endpoints_ipv4
			WHERE
				pair_id=p.pair_id
			) = 0
		ORDER BY
			p.control_ip
	);

COMMIT;
EOSQL

	echo "DONE: ${NETWORK}"
	echo
done
