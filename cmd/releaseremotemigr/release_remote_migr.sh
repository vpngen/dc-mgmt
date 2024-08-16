#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}

PAIRS_SCHEMA=${PAIRS_SCHEMA:-"pairs"}
BRIGADES_SCHEMA=${BRIGADES_SCHEMA:-"brigades"}

printdef() {
        echo "Usage: -f <snapshot_file> [-n] -o <domain_file> [-inet <cidr>] [-enet <cidr>]"
        echo "       -n : dry run"
        echo "       -inet : target control network filter"
        echo "       -enet : target external network filter"

        exit 1
}

while [ $# -gt 0 ]; do
        case "$1" in
                -f)
                        SNAPSHOT_FILE="$2"
                        shift
                        ;;
                        
                -o)
                        DOMAIN_FILE="$2"
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

if [ -z "$SNAPSHOT_FILE" ]; then
        echo "Missing snapshot_file"

        printdef
fi

if [ -z "$DOMAIN_FILE" ]; then
        echo "Missing domain_file"

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

echo "{\"subdomains\": [" > "$DOMAIN_FILE"

# Loop through each item in the JSON array within the "snap" key
jq -c '.snaps[]' < "$SNAPSHOT_FILE" | while read -r snap; do
        brigade_id_32="$(echo "$snap" | jq -r '.brigade_id')"
        brigade_id="$(echo "${brigade_id_32}=========" | base32 -d 2>/dev/null | hexdump -ve '1/1 "%02x"')"

        old_instance="$(psql -d "$DBNAME" -q -t -A \
                -v BRIGADE_ID="$brigade_id" \
                -v BRIGADES_SCHEMA="$BRIGADES_SCHEMA" <<EOF
        SELECT
                b.instance_id,
                b.domain_name
        FROM
                :"BRIGADES_SCHEMA".brigades b
        WHERE
                b.brigade_id = :'BRIGADE_ID'
                AND b.main = true
                AND b.domain_name IS NOT NULL;
EOF
)"

        if [ -z "$old_instance" ]; then
                continue
        fi

        old_instance_id="$(echo "$old_instance" | cut -d '|' -f 1)"
        domain_name="$(echo "$old_instance" | cut -d '|' -f 2)"

        if [ -n "$domain_name" ]; then
                echo "${first}\"${domain_name}\"" >> "$DOMAIN_FILE"
                first=","
        fi

        echo "Brigade: $brigade_id, instance: $old_instance_id"
        echo "         Domain: $domain_name"

        if [ -z "$DRY_RUN" ]; then
                psql -d "$DBNAME" -q -t -A \
                        -v RESERVATION_ID="$RESERVATION_ID" \
                        -v BRIGADE_ID="$brigade_id" \
                        -v BRIGADES_SCHEMA="$BRIGADES_SCHEMA" \
                        -v OLD_INSTANCE_ID="$old_instance_id" \
                        -v DOMAIN_NAME="$domain_name" <<EOF
                BEGIN;
        
                UPDATE 
                        :"BRIGADES_SCHEMA".brigades 
                SET  
                        domain_name = NULL 
                WHERE 
                        brigade_id = :'BRIGADE_ID' 
                        AND instance_id = :'OLD_INSTANCE_ID';

                DELETE FROM 
                        :"BRIGADES_SCHEMA".domains_endpoints_ipv4 
                WHERE 
                        domain_name = :'DOMAIN_NAME';                        

                COMMIT;
EOF
                echo "         Success"
        else
                echo "         Dry runned"
        fi

done

echo "]}" >> "$DOMAIN_FILE"
