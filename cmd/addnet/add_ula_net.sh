#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}
echo "dbname: $DBNAME"
SCHEMA_BRIGADES=${BSCHEMA:-"brigades"}
echo "schema: $SCHEMA_BRIGADES"

ula_net="$1"

if [ "x" = "x${ula_net}" ]; then
    echo "Usage: $0 <ula_net/cidr>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d "${DBNAME}" \
    --set schema_name="${SCHEMA_BRIGADES}" \
    --set ula_net="${ula_net}" <<EOF
BEGIN;
INSERT INTO :"schema_name".ipv6_ula_nets (ipv6_net) VALUES (:'ula_net');
COMMIT;
EOF
