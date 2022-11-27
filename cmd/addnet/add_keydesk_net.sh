#!/bin/sh

set -e

CONFDIR=${CONFDIR:-"/etc/vgrealm"}
echo "confdir: ${CONFDIR}"
DBNAME=${DBNAME:-$(cat ${CONFDIR}/dbname)}
echo "dbname: $DBNAME"
SCHEMA=${SCHEMA:-$(cat ${CONFDIR}/brigades_schema)}
echo "schema: $SCHEMA"

keydesk_net="$1"

if [ "x" = "x${keydesk_net}" ]; then
    echo "Usage: $0 <keydesk_net/cidr>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d ${DBNAME} \
    --set schema_name=${SCHEMA} \
    --set keydesk_net=${keydesk_net} <<EOF
BEGIN;
INSERT INTO :"schema_name".ipv6_keydesk_nets (ipv6_net) VALUES (:'keydesk_net');
COMMIT;
EOF
