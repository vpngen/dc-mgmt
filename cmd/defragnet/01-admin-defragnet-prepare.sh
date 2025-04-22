#!/bin/sh

DB_URL=${DB_URL:-"postgres:///vgrealm"}

set -e

NETWORK=$1

if [ -z "${NETWORK}" ]; then
        echo "no network"
        exit 1
fi

echo "PREPARE NETWORK: ${NETWORK}"

psql "${DB_URL}" -v net="${NETWORK}" <<EOF
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
EOF
