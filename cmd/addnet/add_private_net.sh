#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}
echo "dbname: $DBNAME"
SCHEMA_PAIRS=${PSCHEMA:-"pairs"}
echo "schema: $SCHEMA_PAIRS"

private_net="$1"

if [ "x" = "x${private_net}" ]; then
    echo "Usage: $0 <private_net/cidr>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d "${DBNAME}" \
    --set schema_name="${SCHEMA_PAIRS}" \
    --set private_net="${private_net}" <<EOF
BEGIN;
INSERT INTO :"schema_name".ipv4_nets (ipv4_net) VALUES (:'private_net');
COMMIT;
EOF
