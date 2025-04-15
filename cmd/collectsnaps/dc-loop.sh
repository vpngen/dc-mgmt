#!/bin/sh

DBNAME=${DBNAME:-"vgrealm"}
BRIGADES_SCHEMA=${BRIGADES_SCHEMA:-"brigades"}

nets=$(psql -qtA -d "${DBNAME}" \
        --set ON_ERROR_STOP=yes \
        --set brigades_schema="${BRIGADES_SCHEMA}" <<EOF
SELECT 
        DISTINCT(set_masklen(endpoint_ipv4,24) & '255.255.255.0'::inet) AS net24 
FROM  
        :"brigades_schema".brigades;
EOF
)

basedir=$(dirname "$0")

for net in ${nets}; do
        flock -x -n /tmp/collectsnaps-"${net}".lock "${basedir}"/collectsnaps.sh -tag "periodic-hourly-${net}" -ad -net "${net}/24"
done