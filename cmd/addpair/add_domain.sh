#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}
echo "dbname: $DBNAME"
SCHEMA_PAIRS=${PSCHEMA:-"pairs"}
echo "schema: $SCHEMA_PAIRS"

# shellcheck source=/dev/null
. /etc/vg-dc-mgmt/dc-name.env

ip="$1"

if [ -z "${ip}" ] || [ -z "${domain}" ]; then
    echo "Usage: $0 <ip>"
    exit 1
fi

ON_ERROR_STOP=yes psql -d "${DBNAME}" \
    --set schema_name="${SCHEMA_PAIRS}" \
    --set ip="${ip}" \
    --set domain="${domain}" <<EOF
BEGIN;

INSERT INTO :"schema_name".domains_endpoints_ipv4 (domain_name, endpoint_ipv4) VALUES (:'domain', :'ip');

COMMIT;
EOF

echo "Added domain ${domain} with endpoint ${ip}"