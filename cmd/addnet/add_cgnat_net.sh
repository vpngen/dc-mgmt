#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}
echo "dbname: $DBNAME"
SCHEMA_BRIGADES=${BSCHEMA:-"brigades"}
echo "schema: $SCHEMA_BRIGADES"

cgnat_net="$1"

if [ "x" = "x${cgnat_net}" ]; then
    echo "Usage: $0 <cgnat_net/cidr>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d "${DBNAME}" \
    --set schema_name="${SCHEMA_BRIGADES}" \
    --set cgnat_net="${cgnat_net}" <<EOF
BEGIN;
INSERT INTO :"schema_name".ipv4_cgnat_nets (ipv4_net) VALUES (:'cgnat_net');
COMMIT;
EOF
