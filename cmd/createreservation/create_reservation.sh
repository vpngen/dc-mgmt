#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}

PAIRS_SCHEMA=${PAIRS_SCHEMA:-"pairs"}
BRIGADES_SCHEMA=${BRIGADES_SCHEMA:-"brigades"}


printdef () {
        msg="$1"

        if [ -n "${msg}" ]; then
                echo "ERROR: ${msg}" >&2
        fi
        
        echo "Usage: $0 <command> <options>" >&2
        echo >&2
        echo "Commands:" >&2
        echo "  create [-nc] <number>" >&2
        echo "    -nc Do not generate migration configuration" >&2
        echo "    -in <control network> Control network for filtering" >&2
        echo "    -en <endpoint network> Endpoint network for filtering" >&2
        echo >&2
        echo "  list" >&2
        echo >&2
        echo "  show [-list] <reservation uuid>" >&2
        echo "    -list List reserved slots" >&2
        echo >&2
        echo "  delete [-f] <reservation uuid>" >&2
        echo "    -f Force delete when not empty" >&2
        echo >&2
        echo "  genconf <reservation uuid>" >&2
}

genconf () {
        reservation_uuid="$1"

        echo "Generate configuration for reservation: ${reservation_uuid}" >&2
        echo >&2

        out="{\"plan\":["

        groups=$(psql -d "${DBNAME}" -q -t -A \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set pairs_schema="${PAIRS_SCHEMA}" \
                --set reservation_uuid="${reservation_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT
        p.control_ip,
        p.router_nacl_pubkey
FROM 
        :"brigades_schema".reserved_endpoints_ipv4 r
        JOIN :"pairs_schema".pairs_endpoints_ipv4 pe ON r.endpoint_ipv4 = pe.endpoint_ipv4
        JOIN :"pairs_schema".pairs p ON pe.pair_id = p.pair_id
WHERE
        r.reservation_id = :'reservation_uuid'
GROUP BY p.pair_id;

EOF
)
        group_starting=""
        for group in ${groups}; do
                control_ip=$(echo "${group}" | cut -d'|' -f1)
                router_nacl_pubkey=$(echo "${group}" | cut -d'|' -f2)

                #echo "Control IP: ${control_ip}" >&2
                #echo "Router NACL pubkey: ${router_nacl_pubkey}" >&2
                #echo >&2

                escaped_router_key=$(jq -n --argjson router_nacl_pubkey "${router_nacl_pubkey}" '($router_nacl_pubkey | tostring)')

                out="${out}${group_starting}{\"control_ip\":\"${control_ip}\",\"router_nacl_pubkey\":${escaped_router_key},\"slots\":["
                group_starting=","

                slots=$(psql -d "${DBNAME}" -q -t -A \
                        --set brigades_schema="${BRIGADES_SCHEMA}" \
                        --set pairs_schema="${PAIRS_SCHEMA}" \
                        --set reservation_uuid="${reservation_uuid}" \
                        --set control_ip="${control_ip}" \
                        --set ON_ERROR_STOP=yes  <<EOF
SELECT
        r.endpoint_ipv4
FROM
        :"brigades_schema".reserved_endpoints_ipv4 r
        JOIN :"pairs_schema".pairs_endpoints_ipv4 pe ON r.endpoint_ipv4 = pe.endpoint_ipv4
        JOIN :"pairs_schema".pairs p ON pe.pair_id = p.pair_id
WHERE
        r.reservation_id = :'reservation_uuid'
AND
        p.control_ip = :'control_ip';

EOF
)       
                slot_starting=""
                for slot in ${slots}; do
                        #echo "Control IP: ${control_ip}" >&2
                        #echo "Router NACL pubkey: ${router_nacl_pubkey}" >&2
                        #echo "Endpoint IP: ${ip}" >&2
                        #echo >&2

                        out="${out}${slot_starting}\"${slot}\""
                        slot_starting=","
                done

                out="${out}]}"
        done

        out="${out}]}"

        echo "${out}" | jq
}

create () {
        while [ "$#" -gt 0 ]; do
                case "$1" in
                        -nc)
                                NOCONF=true
                                shift
                                ;;
                        -in)
                                CONTROL_NETWORK="$2"
                                shift 2
                                ;;
                        -en)
                                ENDPOINT_NETWORK="$2"
                                shift 2
                                ;;
                        *)
                                NUMBER="$1"

                                break
                                ;;
                esac
        done

        if [ -z "${NUMBER}" ]; then
                printdef "Number not specified"
                exit 1
        fi

        if ! [ "$NUMBER" -eq "$NUMBER" ] 2>/dev/null; then
                printdef "Number must be numeric"
                exit 1
        fi

        if [ -z "${CONTROL_NETWORK}" ]; then
                CONTROL_NETWORK="0.0.0.0/0"
        fi

        if [ -z "${ENDPOINT_NETWORK}" ]; then
                ENDPOINT_NETWORK="0.0.0.0/0"
        fi

        echo "Control network: ${CONTROL_NETWORK}" >&2
        echo "Endpoint network: ${ENDPOINT_NETWORK}" >&2
        if [ -n "${NOCONF}" ]; then
                echo "Generate configuration: false" >&2
        else
                echo "Generate configuration: true" >&2
        fi


        test=$(psql -d "${DBNAME}" -q -t -A \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set control_network="${CONTROL_NETWORK}" \
                --set endpoint_network="${ENDPOINT_NETWORK}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT 
        COUNT(*)
FROM
        :"brigades_schema".slots s
        JOIN :"brigades_schema".active_pairs p ON s.pair_id = p.pair_id
WHERE
        s.domain_name IS NULL
        AND s.control_ip << :'control_network'
        AND s.endpoint_ipv4 << :'endpoint_network'
EOF
)

        echo "Free slots: ${test}" >&2

        if [ "${test}" -lt "${NUMBER}" ]; then
                echo "Not enough free slots: $((NUMBER-test))" >&2
                exit 1
        fi

        reservation_uuid=$(psql -d "${DBNAME}" -q -t -A \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

INSERT INTO :"brigades_schema".reservations (dismission) VALUES (FALSE) RETURNING reservation_id;

COMMIT;
EOF
)

        echo "Created reservation: ${reservation_uuid}" >&2

        psql -d "${DBNAME}" -q  -t -A\
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set reservation_uuid="${reservation_uuid}" \
                --set control_network="${CONTROL_NETWORK}" \
                --set endpoint_network="${ENDPOINT_NETWORK}" \
                --set number="${NUMBER}" \
                --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

INSERT INTO :"brigades_schema".reserved_endpoints_ipv4 (reservation_id, endpoint_ipv4) 
SELECT 
        :'reservation_uuid',
        s.endpoint_ipv4
FROM
        :"brigades_schema".slots s
        JOIN :"brigades_schema".active_pairs p ON s.pair_id = p.pair_id
WHERE
        s.domain_name IS NULL
        AND s.control_ip << :'control_network'
        AND s.endpoint_ipv4 << :'endpoint_network'
ORDER BY
        p.free_slots_count DESC
LIMIT :'number';

COMMIT;
EOF

        affected=$(psql -d "${DBNAME}" -q -t -A \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set reservation_uuid="${reservation_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT
        COUNT(*)
FROM
        :"brigades_schema".reserved_endpoints_ipv4
WHERE
        reservation_id = :'reservation_uuid'
EOF
)

        echo "Affected rows: ${affected}" >&2
        if [ "${affected}" -ne "${NUMBER}" ]; then
                echo "Not enough free slots: $((NUMBER-affected))" >&2
                exit 1
        fi

        echo "Slots reserved for reservation: ${reservation_uuid}" >&2

        if [ -z "${NOCONF}" ]; then
                genconf "${reservation_uuid}"
        fi
}

show () {
        while [ "$#" -gt 0 ]; do
                case "$1" in
                        -list)
                                DO_LIST="x"
                                shift
                                ;;
                        *)
                                reservation_uuid="$1"

                                break
                                ;;
                esac
        done

        if [ -z "${reservation_uuid}" ]; then
                printdef "Reservation UUID not specified"
                exit 1
        fi

        echo "Reservation UUID: ${reservation_uuid}" >&2
        echo >&2

        psql -d "${DBNAME}" -q\
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set pairs_schema="${PAIRS_SCHEMA}" \
                --set reservation_uuid="${reservation_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT
        r.reservation_id,
        COUNT(CASE WHEN e.endpoint_ipv4 IS NOT NULL THEN 1 END) AS reserved_slots
FROM
        :"brigades_schema".reservations r
        LEFT JOIN :"brigades_schema".reserved_endpoints_ipv4 e ON r.reservation_id = e.reservation_id
WHERE
        r.reservation_id = :'reservation_uuid'
GROUP BY
        r.reservation_id;
EOF

        if [ -n "${DO_LIST}" ]; then
                echo "Reserved slots:" >&2
                echo >&2

                psql -d "${DBNAME}" -q \
                        --set brigades_schema="${BRIGADES_SCHEMA}" \
                        --set pairs_schema="${PAIRS_SCHEMA}" \
                        --set reservation_uuid="${reservation_uuid}" \
                        --set ON_ERROR_STOP=yes  <<EOF
SELECT
        r.endpoint_ipv4,
        p.control_ip
FROM
        :"brigades_schema".reserved_endpoints_ipv4 r
        JOIN :"pairs_schema".pairs_endpoints_ipv4 pe ON r.endpoint_ipv4 = pe.endpoint_ipv4
        JOIN :"pairs_schema".pairs p ON pe.pair_id = p.pair_id
WHERE
        r.reservation_id = :'reservation_uuid'
ORDER BY
        p.control_ip, r.endpoint_ipv4;

EOF
        fi

}

list () {
        psql -d "${DBNAME}" -q \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set ON_ERROR_STOP=yes  <<EOF

SELECT
        r.reservation_id,
        COUNT(CASE WHEN e.endpoint_ipv4 IS NOT NULL THEN 1 END) AS reserved_slots
FROM
        :"brigades_schema".reservations r
        LEFT JOIN :"brigades_schema".reserved_endpoints_ipv4 e ON r.reservation_id = e.reservation_id
GROUP BY
        r.reservation_id
ORDER BY
        r.reservation_id;

EOF
}

delete () {
        while [ "$#" -gt 0 ]; do
                case "$1" in
                        -f)
                                FORCE="x"
                                shift
                                ;;
                        *)
                                reservation_uuid="$1"

                                break
                                ;;
                esac
        done

        if [ -z "${reservation_uuid}" ]; then
                printdef "Reservation UUID not specified"
                exit 1
        fi

        echo "Reservation UUID: ${reservation_uuid}"
        echo "Try to delete reservation: ${reservation_uuid}"

        if [ -z "${FORCE}" ]; then
                psql -d "${DBNAME}" -q \
                        --set brigades_schema="${BRIGADES_SCHEMA}" \
                        --set reservation_uuid="${reservation_uuid}" \
                        --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

DELETE FROM 
        :"brigades_schema".reserved_endpoints_ipv4 
USING :"brigades_schema".reserved_endpoints_ipv4 e
LEFT JOIN :"brigades_schema".brigades b ON e.endpoint_ipv4 = b.endpoint_ipv4
WHERE
        e.reservation_id = :'reservation_uuid'
        AND b.endpoint_ipv4 IS NULL;

DELETE FROM :"brigades_schema".reservations WHERE reservation_id = :'reservation_uuid';

COMMIT;
EOF
        else
                psql -d "${DBNAME}" -q \
                        --set brigades_schema="${BRIGADES_SCHEMA}" \
                        --set reservation_uuid="${reservation_uuid}" \
                        --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

DELETE FROM 
        :"brigades_schema".reserved_endpoints_ipv4 
USING :"brigades_schema".reserved_endpoints_ipv4 e
LEFT JOIN :"brigades_schema".brigades b ON e.endpoint_ipv4 = b.endpoint_ipv4
WHERE
        e.reservation_id = :'reservation_uuid';

DELETE FROM :"brigades_schema".reservations WHERE reservation_id = :'reservation_uuid';

COMMIT;
EOF
        fi

        echo "Deleted reservation: ${reservation_uuid}"

}

COMMAND="$1"
if [ -z "${COMMAND}" ]; then
        printdef "Command not specified"
        exit 1
fi

shift

case "${COMMAND}" in
        create)
                create "$@"
                ;;
        list)
                list "$@"
                ;;
        show)
                show "$@"
                ;;
        delete)
                delete "$@"
                ;;
        genconf)
                genconf "$@"
                ;;
        *)
                printdef "Unknown command: ${COMMAND}"
                exit 1
                ;;
esac


