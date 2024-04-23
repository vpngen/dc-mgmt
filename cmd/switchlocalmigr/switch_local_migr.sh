#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}

PAIRS_SCHEMA=${PAIRS_SCHEMA:-"pairs"}
BRIGADES_SCHEMA=${BRIGADES_SCHEMA:-"brigades"}

if [ -s  "/etc/vg-dc-mgmt/dc-name.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-mgmt/dc-name.env"
fi

if [ -s "/etc/vg-dc-vpnapi/modbrigade.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-vpnapi/modbrigade.env"
fi

printdef() {
        echo "Usage: -r <reservation_id> -f <snapshot_file> [-n] [-inet <cidr>] [-enet <cidr>]"
        echo "       -n : dry run"
        echo "       -inet : target control network filter"
        echo "       -enet : target external network filter"

        exit 1
}

while [ $# -gt 0 ]; do
        case "$1" in
                -r)
                        RESERVATION_ID="$2"
                        shift
                        ;;
                -f)
                        SNAPSHOT_FILE="$2"
                        shift
                        ;;
                -n)
                        DRY_RUN=yes
                        ;;
                -inet)
                        INET_FILTER="$2"
                        shift
                        ;;
                -enet)
                        ENET_FILTER="$2"
                        shift
                        ;;
                *)
                        printdef "Unknown option: $1"
                        ;;
        esac
        shift
done

if [ -z "$RESERVATION_ID" ] || [ -z "$SNAPSHOT_FILE" ]; then
        echo "Missing reservation_id"

        printdef
fi

if [ -z "$SNAPSHOT_FILE" ]; then
        echo "Missing snapshot_file"

        printdef
fi

if [ ! -s "$SNAPSHOT_FILE" ]; then
        echo "Snapshot file not found: $SNAPSHOT_FILE"

        exit 1
fi

if [ -z "$INET_FILTER" ]; then
        INET_FILTER="0.0.0.0/0"
fi

if [ -z "$ENET_FILTER" ]; then
        ENET_FILTER="0.0.0.0/0"
fi

# Loop through each item in the JSON array within the "snap" key
jq -c '.snaps[]' < "$SNAPSHOT_FILE" | while read -r snap; do
        brigade_id_32="$(echo "$snap" | jq -r '.brigade_id')"
        brigade_id="$(echo "${brigade_id_32}=========" | base32 -d 2>/dev/null | hexdump -ve '1/1 "%02x"')"

        new_instance="$(psql -d "$DBNAME" -q -t -A \
                -v RESERVATION_ID="$RESERVATION_ID" \
                -v BRIGADE_ID="$brigade_id" \
                -v PAIRS_SCHEMA="$PAIRS_SCHEMA" \
                -v BRIGADES_SCHEMA="$BRIGADES_SCHEMA" \
                -v INET_FILTER="$INET_FILTER" \
                -v ENET_FILTER="$ENET_FILTER" <<EOF
        SELECT
                b.instance_id,
                b.endpoint_ipv4
        FROM
                :"BRIGADES_SCHEMA".brigades b
                JOIN :"PAIRS_SCHEMA".pairs p ON b.pair_id = p.pair_id
                JOIN :"BRIGADES_SCHEMA".reserved_endpoints_ipv4 re ON b.endpoint_ipv4 = re.endpoint_ipv4
        WHERE
                b.brigade_id = :'BRIGADE_ID'
                AND b.main = false
                AND re.reservation_id = :'RESERVATION_ID'
                AND p.control_ip << :'INET_FILTER'
                AND re.endpoint_ipv4 << :'ENET_FILTER';
EOF
)"
        
        if [ -z "$new_instance" ]; then
                continue
        fi
        
        new_instance_id="$(echo "$new_instance" | cut -d '|' -f 1)"
        new_ipv4="$(echo "$new_instance" | cut -d '|' -f 2)"

        old_instance="$(psql -d "$DBNAME" -q -t -A \
                -v RESERVATION_ID="$RESERVATION_ID" \
                -v BRIGADE_ID="$brigade_id" \
                -v PAIRS_SCHEMA="$PAIRS_SCHEMA" \
                -v BRIGADES_SCHEMA="$BRIGADES_SCHEMA" <<EOF
        SELECT
                b.instance_id,
                b.endpoint_ipv4,
                b.domain_name
        FROM
                :"BRIGADES_SCHEMA".brigades b
                JOIN :"PAIRS_SCHEMA".pairs p ON b.pair_id = p.pair_id
                LEFT JOIN :"BRIGADES_SCHEMA".reserved_endpoints_ipv4 re ON b.endpoint_ipv4 = re.endpoint_ipv4
        WHERE
                b.brigade_id = :'BRIGADE_ID'
                AND b.main = true
                AND b.domain_name IS NOT NULL
                AND re.reservation_id IS NULL;
EOF
)"

        if [ -z "$old_instance" ]; then
                continue
        fi

        old_instance_id="$(echo "$old_instance" | cut -d '|' -f 1)"
        old_ipv4="$(echo "$old_instance" | cut -d '|' -f 2)"
        domain_name="$(echo "$old_instance" | cut -d '|' -f 3)"

        echo "Brigade: $brigade_id, old: $old_instance_id new: $new_instance_id"
        echo "         Domain: $domain_name change from $old_ipv4 to $new_ipv4"

        if [ -z "$DRY_RUN" ]; then
                psql -d "$DBNAME" -q -t -A \
                        -v RESERVATION_ID="$RESERVATION_ID" \
                        -v BRIGADE_ID="$brigade_id" \
                        -v BRIGADES_SCHEMA="$BRIGADES_SCHEMA" \
                        -v OLD_INSTANCE_ID="$old_instance_id" \
                        -v NEW_INSTANCE_ID="$new_instance_id" \
                        -v OLD_IPV4="$old_ipv4" \
                        -v NEW_IPV4="$new_ipv4" \
                        -v DOMAIN_NAME="$domain_name" <<EOF
                BEGIN;
        
                UPDATE 
                        :"BRIGADES_SCHEMA".brigades 
                SET  
                        main = false, 
                        domain_name = NULL 
                WHERE 
                        brigade_id = :'BRIGADE_ID' 
                        AND instance_id = :'OLD_INSTANCE_ID';

                DELETE FROM 
                        :"BRIGADES_SCHEMA".domains_endpoints_ipv4 
                WHERE 
                        endpoint_ipv4 = :'OLD_IPV4';                        

                INSERT INTO 
                        :"BRIGADES_SCHEMA".domains_endpoints_ipv4 
                        (domain_name, endpoint_ipv4) 
                VALUES 
                        (:'DOMAIN_NAME', :'NEW_IPV4');

                UPDATE 
                        :"BRIGADES_SCHEMA".brigades 
                SET  
                        main = true, 
                        domain_name = :'DOMAIN_NAME' 
                WHERE 
                        brigade_id = :'BRIGADE_ID' 
                        AND instance_id = :'NEW_INSTANCE_ID';

                COMMIT;
EOF
                echo "         Success"
        else
                echo "         Dry runned"
        fi

done

SSH_KEY=${SSH_KEY:-"${HOME}/.ssh/id_ed25519"}

DELEGATION_FILENAME="domain-generate-${DC_NAME}.csv"
RELOAD_FILENAME="domain-generate.reload"

domains="$(psql -qtAF ';' -d "${DBNAME}" \
        --set BRIGADES_SCHEMA="${BRIGADES_SCHEMA}" <<EOF
SELECT 
	domain_name,endpoint_ipv4 
FROM 
	:"BRIGADES_SCHEMA".domains_endpoints_ipv4;
EOF
)"

echo "$domains" | ssh -i "${SSH_KEY}" -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "${DELEGATION_SYNC_CONNECT}" \
        "dd status=none of=${DELEGATION_FILENAME}.tmp && mv -f ${DELEGATION_FILENAME}.tmp ${DELEGATION_FILENAME} && touch ${RELOAD_FILENAME}"
