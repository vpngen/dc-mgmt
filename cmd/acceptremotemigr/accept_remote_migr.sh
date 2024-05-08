#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}

PAIRS_SCHEMA=${PAIRS_SCHEMA:-"pairs"}
BRIGADES_SCHEMA=${BRIGADES_SCHEMA:-"brigades"}

printdef() {
        echo "Usage: -r <reservation_id> -f <restore_plan_file> [-n] [-inet <cidr>] [-enet <cidr>]"
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
                        RESTORE_PLAN_FILE="$2"
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

if [ -z "$RESERVATION_ID" ] || [ -z "$RESTORE_PLAN_FILE" ]; then
        echo "Missing reservation_id" >&2

        printdef
fi

if [ -z "$RESTORE_PLAN_FILE" ]; then
        echo "Missing restore plan file" >&2

        printdef
fi

if [ ! -s "$RESTORE_PLAN_FILE" ]; then
        echo "Restore plan file not found: $RESTORE_PLAN_FILE" >&2

        exit 1
fi

file_reservation_id="$(jq -r '.reservation_id' "$RESTORE_PLAN_FILE")"
if [ "$RESERVATION_ID" != "$file_reservation_id" ]; then
        echo "Mismatched reservation_id: $RESERVATION_ID != $file_reservation_id" >&2

        exit 1
fi

if [ -z "$INET_FILTER" ]; then
        INET_FILTER="0.0.0.0/0"
fi

if [ -z "$ENET_FILTER" ]; then
        ENET_FILTER="0.0.0.0/0"
fi

echo "Restoring reservation: $RESERVATION_ID" >&2
echo "Using restore plan file: $RESTORE_PLAN_FILE" >&2
echo "Control network filter: $INET_FILTER" >&2
echo "External network filter: $ENET_FILTER" >&2
echo >&2
if [ -n "$DRY_RUN" ]; then
        echo "Dry run: $DRY_RUN" >&2
fi

echo >&2


cat "$RESTORE_PLAN_FILE" | jq -c '.plan[]' | while read -r NODE; do
        # Loop through each item in the JSON array within the "snap" key
        echo "$NODE" | jq -c '.snaps[]' | while read -r snap; do
                brigade_id_32="$(echo "$snap" | jq -r '.brigade_id')"
                brigade_id="$(echo "${brigade_id_32}=========" | base32 -d 2>/dev/null | hexdump -ve '1/1 "%02x"')"

                endpoint_ipv4="$(echo "$snap" | jq -r '.endpoint_ipv4')"
                domain_name="$(echo "$snap" | jq -r '.domain_names | if . and length > 0 then .[0] else "" end')"

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

                echo "Brigade: $brigade_id, instance: $new_instance_id, ipv4: $new_ipv4, domain: ${domain_name}"

                if [ "$endpoint_ipv4" != "$new_ipv4" ]; then
                        echo "          !!! plan ipv4: $endpoint_ipv4, current ipv4: $new_ipv4" >&2

                        continue
                fi

                echo "$snap" | jq -r '.domain_names | join(" ")' | while read -r dn; do
                        echo "         Domain: '$dn' set to $new_ipv4"
                        if [ -z "$DRY_RUN" ] && [ -n "${dn}" ]; then
                                psql -d "$DBNAME" -q -t -A \
                                        -v ON_ERROR_STOP=1 \
                                        -v BRIGADES_SCHEMA="$BRIGADES_SCHEMA" \
                                        -v NEW_IPV4="$new_ipv4" \
                                        -v DOMAIN_NAME="$dn" <<EOF
BEGIN;

INSERT INTO 
        :"BRIGADES_SCHEMA".domains_endpoints_ipv4 
        (domain_name, endpoint_ipv4) 
VALUES 
        (:'DOMAIN_NAME', :'NEW_IPV4')
ON CONFLICT (domain_name) DO UPDATE
SET 
        endpoint_ipv4 = :'NEW_IPV4';

COMMIT;
EOF
                                echo "         Success"
                        elif [ -z "$DRY_RUN" ] && [ -z "${dn}" ]; then
                                echo "         Emtry domain name, skipped"
                        else
                                echo "         Dry runned"
                        fi

                        break # !!! only one domain name is now supported
                done

                if [ -z "$DRY_RUN" ] && [ -n "${domain_name}" ]; then
                        psql -d "$DBNAME" -q -t -A \
                                -v RESERVATION_ID="$RESERVATION_ID" \
                                -v BRIGADE_ID="$brigade_id" \
                                -v BRIGADES_SCHEMA="$BRIGADES_SCHEMA" \
                                -v NEW_INSTANCE_ID="$new_instance_id" \
                                -v NEW_IPV4="$new_ipv4" \
                                -v DOMAIN_NAME="$domain_name" <<EOF
BEGIN;

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
                elif [ -z "$DRY_RUN" ] && [ -z "${domain_name}" ]; then
                        psql -d "$DBNAME" -q -t -A \
                                -v RESERVATION_ID="$RESERVATION_ID" \
                                -v BRIGADE_ID="$brigade_id" \
                                -v BRIGADES_SCHEMA="$BRIGADES_SCHEMA" \
                                -v NEW_INSTANCE_ID="$new_instance_id" \
                                -v NEW_IPV4="$new_ipv4" <<EOF
BEGIN;

UPDATE 
        :"BRIGADES_SCHEMA".brigades 
SET  
        main = true
WHERE 
        brigade_id = :'BRIGADE_ID' 
        AND instance_id = :'NEW_INSTANCE_ID';

COMMIT;
EOF
                        echo "         Emtry domain name, skipped"                
                else
                        echo "         Dry runned"
                fi
        done
done
