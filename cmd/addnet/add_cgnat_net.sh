#!/bin/sh

set -e

CONFDIR=${CONFDIR:-"/etc/vpngen"}
echo "confdir: ${CONFDIR}"
DBNAME=${DBNAME:-$(cat ${CONFDIR}/dbname)}
echo "dbname: $DBNAME"
SCHEMA=${SCHEMA:-$(cat ${CONFDIR}/brigades_schema)}
echo "schema: $SCHEMA"

cgnat_net="$1"

if [ "x" = "x${cgnat_net}" ]; then
    echo "Usage: $0 <cgnat_net/cidr>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d ${DBNAME} \
    --set schema_name=${SCHEMA} \
    --set cgnat_net=${cgnat_net} <<EOF
BEGIN;
INSERT INTO :"schema_name".ipv4_cgnat_nets (ipv4_net) VALUES (:'cgnat_net');
COMMIT;
EOF
