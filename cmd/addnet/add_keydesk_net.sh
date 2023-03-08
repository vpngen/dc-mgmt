#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}
echo "dbname: $DBNAME"
SCHEMA_BRIGADES=${BSCHEMA:-"brigades"}
echo "schema: $SCHEMA_BRIGADES"

keydesk_net="$1"

if [ "x" = "x${keydesk_net}" ]; then
    echo "Usage: $0 <keydesk_net/cidr>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d "${DBNAME}" \
    --set schema_name="${SCHEMA_BRIGADES}" \
    --set keydesk_net="${keydesk_net}" <<EOF
BEGIN;
INSERT INTO :"schema_name".ipv6_keydesk_nets (ipv6_net) VALUES (:'keydesk_net');
COMMIT;
EOF
