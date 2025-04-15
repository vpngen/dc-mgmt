#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}

PAIRS_SCHEMA=${PAIRS_SCHEMA:-"pairs"}
BRIGADES_SCHEMA=${BRIGADES_SCHEMA:-"brigades"}

printdef() {
        echo "Usage: <command> <options>"  >&2
        echo "  Commands:" >&2
        echo "    switch - promote spare brigades to main" >&2
        echo "    switch -r <replication_id> -f <snapshot_file> [-n] [-inet <cidr>] [-enet <cidr>]" >&2
        echo "    Options:" >&2
        echo "       -r <replication_id> : replication_id" >&2
        echo "       -f <snapshot_file> : snapshot file (may be prepared)" >&2
        echo "       -n : dry run" >&2
        echo "       -inet : target control network filter" >&2
        echo "       -enet : target external network filter" >&2
        echo "    delete - delete spare brigades" >&2
        echo "    delete -r <replication_id> -f <snapshot_file>" >&2
        echo "    Options:" >&2
        echo "       -r <replication_id> : replication_id" >&2
        echo "       -f <snapshot_file> : snapshot file (may be prepared)" >&2
        echo "       -n : dry run" >&2
        echo "       -inet : target control network filter" >&2
        echo "       -enet : target external network filter" >&2

        exit 1
}

switch () {
        while [ $# -gt 0 ]; do
                case "$1" in
                        -r)
                                replication_ID="$2"
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

        if [ -z "$replication_ID" ] || [ -z "$SNAPSHOT_FILE" ]; then
                echo "Missing replication_id" >&2

                printdef
        fi

        if [ -z "$SNAPSHOT_FILE" ]; then
                echo "Missing snapshot_file" >&2

                printdef
        fi

        if [ ! -s "$SNAPSHOT_FILE" ]; then
                echo "Snapshot file not found: $SNAPSHOT_FILE" >&2

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
                        -v replication_ID="$replication_ID" \
                        -v BRIGADE_ID="$brigade_id" \
                        -v INET_FILTER="$INET_FILTER" \
                        -v ENET_FILTER="$ENET_FILTER" <<EOF
SELECT
        b.instance_id,
        b.endpoint_ipv4
FROM
        brigades.brigades b
        JOIN pairs.pairs p ON b.pair_id = p.pair_id
        JOIN brigades.replicated_endpoints_ipv4 re ON b.endpoint_ipv4 = re.endpoint_ipv4
WHERE
        b.brigade_id = :'BRIGADE_ID'
        AND b.main = false
        AND re.src_dst = false
        AND re.replication_id = :'replication_ID'
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
                        -v replication_ID="$replication_ID" \
                        -v BRIGADE_ID="$brigade_id" <<EOF
SELECT
        b.instance_id,
        b.endpoint_ipv4,
        b.domain_name
FROM
        brigades.brigades b
        JOIN pairs.pairs p ON b.pair_id = p.pair_id
        LEFT JOIN brigades.replicated_endpoints_ipv4 re ON b.endpoint_ipv4 = re.endpoint_ipv4
WHERE
        b.brigade_id = :'BRIGADE_ID'
        AND b.main = true
        AND re.src_dst = true
        AND b.domain_name IS NOT NULL
        AND re.replication_id IS NULL;
EOF
)"

                if [ -z "$old_instance" ]; then
                        continue
                fi

                old_instance_id="$(echo "$old_instance" | cut -d '|' -f 1)"
                old_ipv4="$(echo "$old_instance" | cut -d '|' -f 2)"
                domain_name="$(echo "$old_instance" | cut -d '|' -f 3)"

                echo "Brigade: $brigade_id, old: $old_instance_id new: $new_instance_id" >&2
                echo "         Domain: $domain_name change from $old_ipv4 to $new_ipv4" >&2

                if [ -z "$DRY_RUN" ]; then
                        psql -d "$DBNAME" -q -t -A \
                                -v replication_ID="$replication_ID" \
                                -v BRIGADE_ID="$brigade_id" \
                                -v OLD_INSTANCE_ID="$old_instance_id" \
                                -v NEW_INSTANCE_ID="$new_instance_id" \
                                -v OLD_IPV4="$old_ipv4" \
                                -v NEW_IPV4="$new_ipv4" \
                                -v DOMAIN_NAME="$domain_name" <<EOF
BEGIN;
UPDATE 
        brigades.brigades 
SET  
        main = false, 
        domain_name = NULL 
WHERE 
        brigade_id = :'BRIGADE_ID' 
        AND instance_id = :'OLD_INSTANCE_ID';
UPDATE 
        brigades.domains_endpoints_ipv4 
SET
        endpoint_ipv4 = :'NEW_IPV4'
WHERE 
        domain_name = :'DOMAIN_NAME';                        
UPDATE 
        brigades.brigades 
SET  
        main = true, 
        domain_name = :'DOMAIN_NAME' 
WHERE 
        brigade_id = :'BRIGADE_ID' 
        AND instance_id = :'NEW_INSTANCE_ID';
COMMIT;
EOF
                        echo "         Success" >&2
                else
                        echo "         Dry runned" >&2
                fi

        done

        exit 0
}

delete () {
        while [ $# -gt 0 ]; do
                case "$1" in
                        -r)
                                replication_ID="$2"
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

        if [ -z "$replication_ID" ] || [ -z "$SNAPSHOT_FILE" ]; then
                echo "Missing replication_id" >&2

                printdef
        fi

        if [ -z "$SNAPSHOT_FILE" ]; then
                echo "Missing snapshot_file" >&2

                printdef
        fi

        if [ ! -s "$SNAPSHOT_FILE" ]; then
                echo "Snapshot file not found: $SNAPSHOT_FILE" >&2

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
                        -v replication_ID="$replication_ID" \
                        -v BRIGADE_ID="$brigade_id" \
                        -v INET_FILTER="$INET_FILTER" \
                        -v ENET_FILTER="$ENET_FILTER" <<EOF
SELECT
        b.instance_id
FROM
        brigades.brigades b
        JOIN pairs.pairs p ON b.pair_id = p.pair_id
        JOIN brigades.replicated_endpoints_ipv4 re ON b.endpoint_ipv4 = re.endpoint_ipv4
WHERE
        b.brigade_id = :'BRIGADE_ID'
        AND b.main = true
        AND re.src_dst = false
        AND re.replication_id = :'replication_ID'
        AND p.control_ip << :'INET_FILTER'
        AND re.endpoint_ipv4 << :'ENET_FILTER';
EOF
)"
                
                if [ -z "$new_instance" ]; then
                        echo "Brigade: $brigade_id, no brigades in replication found" >&2

                        continue
                fi
                
                new_instance_id="$(echo "$new_instance" | cut -d '|' -f 1)"

                old_instance="$(psql -d "$DBNAME" -q -t -A \
                        -v BRIGADE_ID="$brigade_id" <<EOF
SELECT
        b.instance_id
FROM
        brigades.brigades b
        JOIN pairs.pairs p ON b.pair_id = p.pair_id
        LEFT JOIN brigades.replicated_endpoints_ipv4 re ON b.endpoint_ipv4 = re.endpoint_ipv4
WHERE
        b.brigade_id = :'BRIGADE_ID'
        AND b.main = false
        AND re.src_dst = true
        AND re.replication_id IS NULL;
EOF
)"

                if [ -z "$old_instance" ]; then
                        echo "Brigade: $brigade_id, no spare brigades found" >&2

                        continue
                fi

                old_instance_id="$(echo "$old_instance" | cut -d '|' -f 1)"

                echo "Brigade: $brigade_id, old: $old_instance_id new: $new_instance_id" >&2

                if [ -z "$DRY_RUN" ]; then
                        psql -d "$DBNAME" -q -t -A \
                                -v BRIGADE_ID="$brigade_id" \
                                -v OLD_INSTANCE_ID="$old_instance_id" << EOF
BEGIN;
DELETE FROM
        brigades.brigades
WHERE
        main = false
        AND brigade_id = :'BRIGADE_ID'
        AND instance_id = :'OLD_INSTANCE_ID'
;

COMMIT;
EOF
                        echo "         Success"
                else
                        echo "         Dry runned"
                fi

        done

        exit 0
}

purge () {
        while [ $# -gt 0 ]; do
                case "$1" in
                        -r)
                                replication_ID="$2"
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

        if [ -z "$replication_ID" ] || [ -z "$SNAPSHOT_FILE" ]; then
                echo "Missing replication_id" >&2

                printdef
        fi

        if [ -z "$SNAPSHOT_FILE" ]; then
                echo "Missing snapshot_file" >&2

                printdef
        fi

        if [ ! -s "$SNAPSHOT_FILE" ]; then
                echo "Snapshot file not found: $SNAPSHOT_FILE" >&2

                exit 1
        fi

        if [ -z "$INET_FILTER" ]; then
                INET_FILTER="0.0.0.0/0"
        fi

        if [ -z "$ENET_FILTER" ]; then
                ENET_FILTER="0.0.0.0/0"
        fi

        if [ -n "$DRY_RUN" ]; then
                echo "Dry runned" >&2
                echo  >&2
        fi

        # Loop through each item in the JSON array within the "snap" key
        jq -c '.snaps[]' < "$SNAPSHOT_FILE" | while read -r snap; do
                brigade_id_32="$(echo "$snap" | jq -r '.brigade_id')"
                brigade_id="$(echo "${brigade_id_32}=========" | base32 -d 2>/dev/null | hexdump -ve '1/1 "%02x"')"

                new_instance="$(psql -d "$DBNAME" -q -t -A \
                        -v replication_ID="$replication_ID" \
                        -v BRIGADE_ID="$brigade_id" \
                        -v INET_FILTER="$INET_FILTER" \
                        -v ENET_FILTER="$ENET_FILTER" <<EOF
SELECT
        b.instance_id
FROM
        brigades.brigades b
        JOIN pairs.pairs p ON b.pair_id = p.pair_id
        JOIN brigades.replicated_endpoints_ipv4 re ON b.endpoint_ipv4 = re.endpoint_ipv4
WHERE
        b.brigade_id = :'BRIGADE_ID'
        AND b.main = true
        AND re.replication_id = :'replication_ID'
        AND p.control_ip << :'INET_FILTER'
        AND re.endpoint_ipv4 << :'ENET_FILTER';
EOF
)"
                
                if [ -z "$new_instance" ]; then
                        echo "Brigade: $brigade_id, no brigades in replication found" >&2

                        continue
                fi
                
                new_instance_id="$(echo "$new_instance" | cut -d '|' -f 1)"

                old_instance="$(psql -d "$DBNAME" -q -t -A \
                        -v BRIGADE_ID="$brigade_id" <<EOF
SELECT
        b.instance_id,p.control_ip
FROM
        brigades.brigades b
        JOIN pairs.pairs p ON b.pair_id = p.pair_id
        LEFT JOIN brigades.replicated_endpoints_ipv4 re ON b.endpoint_ipv4 = re.endpoint_ipv4
WHERE
        b.brigade_id = :'BRIGADE_ID'
        AND b.main = false
        AND re.replication_id IS NULL;
EOF
)"

                if [ -z "$old_instance" ]; then
                        echo "Brigade: $brigade_id, no spare brigades found" >&2

                        continue
                fi

                old_instance_id="$(echo "$old_instance" | cut -d '|' -f 1)"
                control_ip="$(echo "$old_instance" | cut -d '|' -f 2)"

                echo "Brigade: $brigade_id, old: $old_instance_id new: $new_instance_id" >&2

                bid="$(echo "${brigade_id}" | xxd -r -p -l 16 | base32 | tr -d "=")"
                echo "_serega_@${control_ip} destroy -force -id ${bid}"

        done

        exit 0
}

while [ $# -gt 0 ]; do
        cmd="$1"
        shift
        case "$cmd" in
                switch)
                        switch "$@"
                        ;;
                delete)
                        delete "$@"
                        ;;
                purge)
                        purge "$@"
                        ;;
                -h|--help)
                        printdef
                        ;;
                *)
                        printdef "Unknown command: $1"
                        ;;
        esac
done