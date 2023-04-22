#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}
echo "dbname: $DBNAME"
SCHEMA_PAIRS=${PSCHEMA:-"pairs"}
echo "schema: $SCHEMA_PAIRS"

ipv4_net="$1"
gateway="$2"

if [ "x" = "x${ipv4_net}" -o "x" = "x${gateway}" ]; then
    echo "Usage: $0 <ipv4_net/cidr> <gateway>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d "${DBNAME}" \
    --set schema_name="${SCHEMA_PAIRS}" \
    --set ipv4_net="${ipv4_net}" \
    --set gateway="${gateway}"/32 <<EOF
BEGIN;
INSERT INTO :"schema_name".ipv4_nets (ipv4_net, gateway) VALUES (:'ipv4_net', :'gateway');
COMMIT;
EOF
