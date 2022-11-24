#!/bin/sh

set -e

CONFDIR=${CONFDIR:-"/etc/vpngen"}
echo "confdir: ${CONFDIR}"
DBNAME=${DBNAME:-$(cat ${CONFDIR}/dbname)}
echo "dbname: $DBNAME"
SCHEMA=${SCHEMA:-$(cat ${CONFDIR}/pairs_schema)}
echo "schema: $SCHEMA"

pair_id="$1"
control_ip="$2"
shift; shift


if [ "x" = "x${pair_id}" -o "x" = "x${control_ip}" ]; then
    echo "Usage: $0 <pair_id> <control_ip> <external ip>..."
    exit 1
fi

for ep in $@ ; do
    endpoints="${endpoints}
INSERT INTO :\"schema_name\".pairs_endpoints_ipv4 (pair_id, endpoint_ipv4) VALUES (:'pair_id', '${ep}');"
done

ON_ERROR_STOP=yes psql -v -a -d ${DBNAME} \
    --set schema_name=${SCHEMA} \
    --set pair_id=${pair_id} \
    --set control_ip=${control_ip} <<EOF
BEGIN;

INSERT INTO :"schema_name".pairs (pair_id,control_ip,is_active) VALUES (:'pair_id', :'control_ip', false);
${endpoints}

WITH qid AS (
    INSERT INTO :"schema_name".pairs_queue (payload) VALUES ( '{ "cmd":"new-pair", "pair_id":"':'pair_id''"}' :: json ) RETURNING queue_id
)
SELECT pg_notify('qpairs', (SELECT queue_id FROM qid) :: text);

COMMIT;
EOF
