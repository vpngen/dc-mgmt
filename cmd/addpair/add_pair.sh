#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}
echo "dbname: $DBNAME"

pair_id="$1"
control_ip="$2"
srvzone="$3"
shift; shift; shift

if [ -z "${pair_id}" ] || [ -z "${control_ip}"  ] || [ -z "${srvzone}" ]; then
    echo "Usage: $0 <pair_id> <control_ip> <external ip>..."
    exit 1
fi

if [ "${srvzone}" = " " ]; then
        srvzone=""
fi


for ep in "$@" ; do
    endpoints="${endpoints}
INSERT INTO 
        pairs.pairs_endpoints_ipv4 
                (pair_id, endpoint_ipv4) 
        SELECT
                pair_id, '${ep}'
        FROM 
                pairs.pairs
        WHERE 
                control_ip = :'control_ip'
ON CONFLICT (endpoint_ipv4) DO NOTHING
;"
done

ON_ERROR_STOP=yes psql -v -a -d "${DBNAME}" \
    --set zone="${srvzone}" \
    --set pair_id="${pair_id}" \
    --set control_ip="${control_ip}" <<EOF
BEGIN;

INSERT INTO 
        pairs.isolation_groups 
                (description, update_time)
        SELECT 
                network(set_masklen(:'control_ip', 24))::text, NOW() AT TIME ZONE 'UTC'
        WHERE NOT EXISTS (
                SELECT 
                        1
                FROM 
                        pairs.isolation_groups
                WHERE 
                        description = network(set_masklen(:'control_ip', 24))::text
        )
;

INSERT INTO 
        pairs.pairs 
                (control_ip, zone, is_active, pair_id, igrp_id)
        SELECT 
                :'control_ip', :'zone', false, :'pair_id', igrp_id
        FROM
                pairs.isolation_groups
        WHERE   
                description = network(set_masklen(:'control_ip', 24))::text
ON CONFLICT (control_ip) DO UPDATE
        SET
                igrp_id = EXCLUDED.igrp_id,
                zone = EXCLUDED.zone
;
${endpoints}


COMMIT;
EOF
