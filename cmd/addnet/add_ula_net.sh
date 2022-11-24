#!/bin/sh

set -e

CONFDIR=${CONFDIR:-"/etc/vpngen"}
echo "confdir: ${CONFDIR}"
DBNAME=${DBNAME:-$(cat ${CONFDIR}/dbname)}
echo "dbname: $DBNAME"
SCHEMA=${SCHEMA:-$(cat ${CONFDIR}/brigades_schema)}
echo "schema: $SCHEMA"

ula_net="$1"

if [ "x" = "x${ula_net}" ]; then
    echo "Usage: $0 <ula_net/cidr>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d ${DBNAME} \
    --set schema_name=${SCHEMA} \
    --set ula_net=${ula_net} <<EOF
BEGIN;
INSERT INTO :"schema_name".ipv6_ula_nets (ipv6_net) VALUES (:'ula_net');
COMMIT;
EOF
