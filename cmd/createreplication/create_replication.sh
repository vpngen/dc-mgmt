#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}

printdef () {
        msg="$1"

        if [ -n "${msg}" ]; then
                echo "ERROR: ${msg}" >&2
        fi
        
        echo "Usage: $0 <command> <options>" >&2
        echo >&2
        echo "Commands:" >&2
        echo "  create" >&2
        echo "    -src-in <control network> Control network for source filtering" >&2
        echo "    -src-en <endpoint network> Endpoint network for source filtering" >&2
        echo "    -dst-in <control network> Control network for destination filtering" >&2
        echo "    -dst-en <endpoint network> Endpoint network for destination filtering" >&2
        echo >&2
        echo "  list" >&2
        echo >&2
        echo "  show [-list] <replication uuid>" >&2
        echo "    -list List replicated slots" >&2
        echo >&2
        echo "  delete [-f] <replication uuid>" >&2
        echo "    -f Force delete when not empty" >&2
        echo >&2
        echo "  clear <replication uuid>" >&2
        echo >&2
        echo "  swap <replication uuid>" >&2
        echo >&2
        echo "  genconf <replication uuid>" >&2
}

UNASSIGNED_BRIGADES=""
POPPED_BRIGADE=""

pop_value() {
  if [ -z "${UNASSIGNED_BRIGADES}" ]; then
    POPPED_BRIGADE=""

    return
  fi

  # Save $UNASSIGNED_BRIGADES contents into positional parameters
  set -- ${UNASSIGNED_BRIGADES}
  # The "popped" value is the first positional parameter
  POPPED_BRIGADE="$1"
  # Shift off the first positional parameter
  shift
  # Rebuild UNASSIGNED_BRIGADES with remaining items
  UNASSIGNED_BRIGADES="$*"
  # Print the popped value
  #echo "${POPPED_BRIGADE}" >&2
}

genmap () {
        replication_uuid="$1"

        echo "Generate configuration for replication: ${replication_uuid}" >&2
        echo >&2

        UNASSIGNED_BRIGADES="$(psql -d "${DBNAME}" -q -t -A \
                --set replication_uuid="${replication_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
        SELECT
                b.brigade_id
        FROM
                brigades.replicated_endpoints_ipv4 re
                JOIN brigades.brigades b ON re.endpoint_ipv4 = b.endpoint_ipv4
                LEFT JOIN brigades.brigades dst ON b.brigade_id = dst.brigade_id AND b.main = false
                LEFT JOIN brigades.replicated_endpoints_ipv4 redst ON dst.endpoint_ipv4 = redst.endpoint_ipv4 AND redst.replication_id = :'replication_uuid' AND redst.src_dst = false
        WHERE
                re.replication_id = :'replication_uuid'
        AND
                re.src_dst = true
        AND
                dst.brigade_id IS NULL        
;
EOF
)"

        #echo "UNASSIGNED_BRIGADES: ${UNASSIGNED_BRIGADES}" >&2

        #out="{\"replication_id\":\"${replication_uuid}\", \"plan\":["
        out="{"

        groups=$(psql -d "${DBNAME}" -q -t -A \
                --set replication_uuid="${replication_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT
        p.control_ip,
        p.router_nacl_pubkey
FROM 
        brigades.replicated_endpoints_ipv4 r
        JOIN pairs.pairs_endpoints_ipv4 pe ON r.endpoint_ipv4 = pe.endpoint_ipv4
        JOIN pairs.pairs p ON pe.pair_id = p.pair_id
WHERE
        r.replication_id = :'replication_uuid'
        AND r.src_dst = false
GROUP BY 
        p.pair_id
ORDER BY 
        p.control_ip ASC;

EOF
)
        #group_starting=""
        slot_starting=""
        for group in ${groups}; do
                control_ip=$(echo "${group}" | cut -d'|' -f1)
                #router_nacl_pubkey=$(echo "${group}" | cut -d'|' -f2)

                #echo "Control IP: ${control_ip}" >&2
                #echo "Router NACL pubkey: ${router_nacl_pubkey}" >&2
                #echo >&2

                #escaped_router_key=$(jq -n --argjson router_nacl_pubkey "${router_nacl_pubkey}" '($router_nacl_pubkey | tostring)')

                #out="${out}${group_starting}{\"control_ip\":\"${control_ip}\",\"router_nacl_pubkey\":${escaped_router_key},\"slots\":["
                #group_starting=","

                slots=$(psql -d "${DBNAME}" -q -t -A \
                        --set replication_uuid="${replication_uuid}" \
                        --set control_ip="${control_ip}" \
                        --set ON_ERROR_STOP=yes  <<EOF
SELECT
        r.endpoint_ipv4
FROM
        brigades.replicated_endpoints_ipv4 r
        JOIN pairs.pairs_endpoints_ipv4 pe ON r.endpoint_ipv4 = pe.endpoint_ipv4
        JOIN pairs.pairs p ON pe.pair_id = p.pair_id
WHERE
        r.replication_id = :'replication_uuid'
AND
        p.control_ip = :'control_ip'
ORDER BY 
        r.endpoint_ipv4 ASC;

EOF
)       
                for slot in ${slots}; do
                        #echo "Control IP: ${control_ip}" >&2
                        #echo "Router NACL pubkey: ${router_nacl_pubkey}" >&2
                        #echo "Endpoint IP: ${ip}" >&2
                        #echo >&2

                        #echo "SLOT: ${slot}" >&2

                        pop_value
                        #echo "BRIGADE: ${POPPED_BRIGADE}" >&2

                        if [ -z "${POPPED_BRIGADE}" ]; then
                                break
                        fi

                        brigade_id="$(echo "${POPPED_BRIGADE}" | xxd -r -p -l 16 | base32 | tr -d "=")"

                        #out="${out}${slot_starting}{\"${slot}\":\"${brigade_id}\"}"
                        out="${out}${slot_starting}\"${brigade_id}\":\"${slot}\""
                        slot_starting=","
                done

                maps="$(psql -d "${DBNAME}" -q -t -A \
                        --set replication_uuid="${replication_uuid}" \
                        --set control_ip="${control_ip}" \
                        --set ON_ERROR_STOP=yes  <<EOF
SELECT
        b.endpoint_ipv4, b.brigade_id
FROM
        brigades.replicated_endpoints_ipv4 re
        JOIN brigades.brigades b ON re.endpoint_ipv4 = b.endpoint_ipv4
        JOIN pairs.pairs p ON b.pair_id = p.pair_id
WHERE
        re.replication_id = :'replication_uuid'
AND
        p.control_ip = :'control_ip'
AND
        re.src_dst = false
;
EOF
)"

                for map in ${maps}; do
                        #echo "MAP: ${map}" >&2

                        slot=$(echo "${map}" | cut -d'|' -f1)
                        brigade_id="$(echo "${map}" | cut -d'|' -f2 | xxd -r -p -l 16 | base32 | tr -d "=")"
 
                        #out="${out}${slot_starting}{\"${slot}\":\"${brigade_id}\"}"
                        out="${out}${slot_starting}\"${brigade_id}\":\"${slot}\""
                        slot_starting=","
                done
                        
                #out="${out}]}"
        done

        #out="${out}]}"
        out="${out}}"

        echo "${out}" #| jq
}

genconf () {
        replication_uuid="$1"

        echo "Generate configuration for replication: ${replication_uuid}" >&2
        echo >&2

        out="{\"replication_id\":\"${replication_uuid}\", \"reservation_id\":\"${replication_uuid}\", \"plan\":["

        groups=$(psql -d "${DBNAME}" -q -t -A \
                --set replication_uuid="${replication_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT
        p.control_ip,
        p.router_nacl_pubkey
FROM 
        brigades.replicated_endpoints_ipv4 r
        JOIN pairs.pairs_endpoints_ipv4 pe ON r.endpoint_ipv4 = pe.endpoint_ipv4
        JOIN pairs.pairs p ON pe.pair_id = p.pair_id
WHERE
        r.replication_id = :'replication_uuid'
        AND r.src_dst = false
GROUP BY 
        p.pair_id
ORDER BY 
        p.control_ip ASC;

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
                        --set replication_uuid="${replication_uuid}" \
                        --set control_ip="${control_ip}" \
                        --set ON_ERROR_STOP=yes  <<EOF
SELECT
        r.endpoint_ipv4
FROM
        brigades.replicated_endpoints_ipv4 r
        JOIN pairs.pairs_endpoints_ipv4 pe ON r.endpoint_ipv4 = pe.endpoint_ipv4
        JOIN pairs.pairs p ON pe.pair_id = p.pair_id
WHERE
        r.replication_id = :'replication_uuid'
AND
        p.control_ip = :'control_ip'
ORDER BY 
        r.endpoint_ipv4 ASC;

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
                        -src-in)
                                SOURCE_CONTROL_NETWORK="$2"
                                shift 2
                                ;;
                        -src-en)
                                SOURCE_ENDPOINT_NETWORK="$2"
                                shift 2
                                ;;
                        -dst-in)
                                DESTINATION_CONTROL_NETWORK="$2"
                                shift 2
                                ;;
                        -dst-en)
                                DESTINATION_ENDPOINT_NETWORK="$2"
                                shift 2
                                ;;
                        *)
                                printdef "Unknown option: $1"

                                exit 1
                                ;;
                esac
        done

        if [ -z "${SOURCE_CONTROL_NETWORK}" ]; then
                SOURCE_CONTROL_NETWORK="0.0.0.0/0"
        fi

        if [ -z "${SOURCE_ENDPOINT_NETWORK}" ]; then
                SOURCE_ENDPOINT_NETWORK="0.0.0.0/0"
        fi

        if [ -z "${DESTINATION_CONTROL_NETWORK}" ]; then
                DESTINATION_CONTROL_NETWORK="0.0.0.0/0"
        fi

        if [ -z "${DESTINATION_ENDPOINT_NETWORK}" ]; then
                DESTINATION_ENDPOINT_NETWORK="0.0.0.0/0"
        fi

        echo "Source Control network: ${SOURCE_CONTROL_NETWORK}" >&2
        echo "Source Endpoint network: ${SOURCE_ENDPOINT_NETWORK}" >&2
        echo "Destination Control network: ${DESTINATION_CONTROL_NETWORK}" >&2
        echo "Destination Endpoint network: ${DESTINATION_ENDPOINT_NETWORK}" >&2

        test_merge=$(psql -d "${DBNAME}" -q -t -A \
                --set source_control_network="${SOURCE_CONTROL_NETWORK}" \
                --set source_endpoint_network="${SOURCE_ENDPOINT_NETWORK}" \
                --set destination_control_network="${DESTINATION_CONTROL_NETWORK}" \
                --set destination_endpoint_network="${DESTINATION_ENDPOINT_NETWORK}" \
                --set ON_ERROR_STOP=yes  <<EOF
        SELECT 
                CASE
                        WHEN (
                                (
                                        (:'source_control_network'::inet != '0.0.0.0/0') AND (:'destination_control_network'::inet != '0.0.0.0/0')
                                ) AND (
                                        (:'source_control_network'::inet <<= :'destination_control_network'::inet) OR
                                        (:'destination_control_network'::inet <<= :'source_control_network'::inet)
                                )
                        ) OR (
                                (
                                        (:'source_endpoint_network'::inet != '0.0.0.0/0') AND (:'destination_endpoint_network'::inet != '0.0.0.0/0')
                                ) AND (
                                        (:'destination_endpoint_network'::inet <<= :'source_endpoint_network'::inet) OR
                                        (:'source_endpoint_network'::inet <<= :'destination_endpoint_network'::inet) 
                                )
                        )
                        THEN
                                0
                        ELSE
                                1
                END
        ;
EOF
)

        if [ "${test_merge}" -eq "0" ]; then
                echo "Source and destination intersects" >&2
                exit 1
        fi

        test_src=$(psql -d "${DBNAME}" -q -t -A \
                --set source_control_network="${SOURCE_CONTROL_NETWORK}" \
                --set source_endpoint_network="${SOURCE_ENDPOINT_NETWORK}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT 
        COUNT(*)
FROM
        pairs.pairs_endpoints_ipv4 pei
        JOIN pairs.pairs p ON pei.pair_id = p.pair_id
WHERE
        p.control_ip <<= :'source_control_network'
        AND pei.endpoint_ipv4 <<= :'source_endpoint_network'
EOF
)

        echo "Source slots: ${test_src}" >&2

        test_dst=$(psql -d "${DBNAME}" -q -t -A \
                --set destination_control_network="${DESTINATION_CONTROL_NETWORK}" \
                --set destination_endpoint_network="${DESTINATION_ENDPOINT_NETWORK}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT 
        COUNT(*)
FROM
        pairs.pairs_endpoints_ipv4 pei
        JOIN pairs.pairs p ON pei.pair_id = p.pair_id
WHERE
        p.control_ip <<= :'destination_control_network'
        AND pei.endpoint_ipv4 <<= :'destination_endpoint_network'
EOF
)

        echo "Destination slots: ${test_dst}" >&2


        if [ "${test_dst}" -lt "${test_src}" ]; then
                echo "Source bigger than desination" >&2
                exit 1
        fi

        replication_uuid=$(psql -d "${DBNAME}" -q -t -A \
                --set src_filter="-src-in ${SOURCE_CONTROL_NETWORK} -src-en ${SOURCE_ENDPOINT_NETWORK}" \
                --set dst_filter="-dst-in ${SOURCE_CONTROL_NETWORK} -dst-en ${SOURCE_ENDPOINT_NETWORK}" \
                --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

INSERT INTO brigades.replications (src_filter, dst_filter) VALUES (:'src_filter', :'dst_filter') RETURNING replication_id;

COMMIT;
EOF
)

        echo "Created replication: ${replication_uuid}" >&2

        psql -d "${DBNAME}" -q  -t -A\
                --set replication_uuid="${replication_uuid}" \
                --set source_control_network="${SOURCE_CONTROL_NETWORK}" \
                --set source_endpoint_network="${SOURCE_ENDPOINT_NETWORK}" \
                --set destination_control_network="${DESTINATION_CONTROL_NETWORK}" \
                --set destination_endpoint_network="${DESTINATION_ENDPOINT_NETWORK}" \
                --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

INSERT INTO brigades.replicated_endpoints_ipv4 (replication_id, endpoint_ipv4, src_dst) 
SELECT 
        :'replication_uuid',
        pei.endpoint_ipv4,
        true
FROM
        pairs.pairs_endpoints_ipv4 pei
        JOIN pairs.pairs p ON pei.pair_id = p.pair_id
WHERE
        p.control_ip <<= :'source_control_network'
        AND pei.endpoint_ipv4 <<= :'source_endpoint_network'
;

INSERT INTO brigades.replicated_endpoints_ipv4 (replication_id, endpoint_ipv4, src_dst) 
SELECT 
        :'replication_uuid',
        pei.endpoint_ipv4,
        false
FROM
        pairs.pairs_endpoints_ipv4 pei
        JOIN pairs.pairs p ON pei.pair_id = p.pair_id
WHERE
        p.control_ip <<= :'destination_control_network'
        AND pei.endpoint_ipv4 <<= :'destination_endpoint_network'
;

COMMIT;
EOF

        affected_src=$(psql -d "${DBNAME}" -q -t -A \
                --set replication_uuid="${replication_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT
        COUNT(*)
FROM
        brigades.replicated_endpoints_ipv4
WHERE
        replication_id = :'replication_uuid'
        AND src_dst = true
EOF
)

        affected_dst=$(psql -d "${DBNAME}" -q -t -A \
                --set replication_uuid="${replication_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT
        COUNT(*)
FROM
        brigades.replicated_endpoints_ipv4
WHERE
        replication_id = :'replication_uuid'
        AND src_dst = false
EOF
)


        echo "Affected rows: source: ${affected_src} destination: ${affected_dst}" >&2
        echo "Slots replicated for replication: ${replication_uuid}" >&2
}

clear () {
        while [ "$#" -gt 0 ]; do
                case "$1" in
                        -list)
                                DO_LIST="x"
                                shift
                                ;;
                        *)
                                replication_uuid="$1"

                                break
                                ;;
                esac
        done

        if [ -z "${replication_uuid}" ]; then
                printdef "replication UUID not specified"
                exit 1
        fi

        echo "CLEAR replication UUID dest: ${replication_uuid}" >&2
        echo >&2

        psql -d "${DBNAME}" -q \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set stats_schema="${STATS_SCHEMA}" \
                --set replication_uuid="${replication_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

DELETE FROM 
        stats.brigades_stats
USING 
        brigades.brigades b,
        brigades.replicated_endpoints_ipv4 re
WHERE
        re.replication_id = :'replication_uuid'
        AND (brigades_stats.brigade_id = b.brigade_id AND brigades_stats.instance_id = b.instance_id)
        AND b.endpoint_ipv4 = re.endpoint_ipv4
        AND b.main = false;

DELETE FROM 
        brigades.brigades 
USING 
        brigades.replicated_endpoints_ipv4 re
WHERE
        re.replication_id = :'replication_uuid',
        AND re.src_dst = false
        AND brigades.endpoint_ipv4 = re.endpoint_ipv4
        AND brigades.main = false;

COMMIT;
EOF
}


show () {
        while [ "$#" -gt 0 ]; do
                case "$1" in
                        -list)
                                DO_LIST="x"
                                shift
                                ;;
                        *)
                                replication_uuid="$1"

                                break
                                ;;
                esac
        done

        if [ -z "${replication_uuid}" ]; then
                printdef "replication UUID not specified"
                exit 1
        fi

        echo "replication UUID: ${replication_uuid}" >&2
        echo >&2

        psql -d "${DBNAME}" -q\
                --set replication_uuid="${replication_uuid}" \
                --set ON_ERROR_STOP=yes  <<EOF
SELECT
        r.replication_id,
        COUNT(CASE WHEN e.endpoint_ipv4 IS NOT NULL THEN 1 END) AS replicated_slots,
        r.src_filter AS src_filter,
        r.dst_filter AS dst_filter
FROM
        brigades.replications r
        LEFT JOIN brigades.replicated_endpoints_ipv4 e ON r.replication_id = e.replication_id
WHERE
        r.replication_id = :'replication_uuid'
GROUP BY
        r.replication_id, r.src_filter, r.dst_filter;
EOF

        if [ -n "${DO_LIST}" ]; then
                echo "replicated slots:" >&2
                echo >&2

                psql -d "${DBNAME}" -q \
                        --set brigades_schema="${BRIGADES_SCHEMA}" \
                        --set pairs_schema="${PAIRS_SCHEMA}" \
                        --set replication_uuid="${replication_uuid}" \
                        --set ON_ERROR_STOP=yes  <<EOF
SELECT
        r.endpoint_ipv4,
        p.control_ip,
        r.src_dst
FROM
        brigades.replicated_endpoints_ipv4 r
        JOIN pairs.pairs_endpoints_ipv4 pe ON r.endpoint_ipv4 = pe.endpoint_ipv4
        JOIN pairs.pairs p ON pe.pair_id = p.pair_id
WHERE
        r.replication_id = :'replication_uuid'
ORDER BY
        p.src_dst, p.control_ip, r.endpoint_ipv4;

EOF
        fi

}

list () {
        psql -d "${DBNAME}" -q \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set ON_ERROR_STOP=yes  <<EOF

SELECT
        r.replication_id,
        COUNT(CASE WHEN e.endpoint_ipv4 IS NOT NULL THEN 1 END) AS replicated_slots,
        r.src_filter AS src_filter,
        r.dst_filter AS dst_filter
FROM
        brigades.replications r
        LEFT JOIN brigades.replicated_endpoints_ipv4 e ON r.replication_id = e.replication_id AND e.src_dst = false
GROUP BY
        r.replication_id, r.src_filter, r.dst_filter
ORDER BY
        r.replication_id;

EOF
}


filter_string=""

split_filter () {
        src_in=""
        src_en=""
        replication_id="$1"

        if [ -z "${replication_id}" ]; then
                printdef "replication UUID not specified"
                exit 1
        fi

        set -- $filter_string

        while [ "$#" -gt 0 ]; do
        case "$1" in
                -src-in)
                src_in="$2"
                shift 2
                ;;
                -src-en)
                src_en="$2"
                shift 2
                ;;
                *)
                shift
                ;;
        esac
        done

        echo "${replication_id}|${src_in}|${src_en}"
}

auto_list () {
        list="$(psql -d "${DBNAME}" -qtA --set ON_ERROR_STOP=yes  <<EOF

SELECT
        r.replication_id
FROM
        brigades.replications r
ORDER BY
        r.replication_id;

EOF
)"

        for line in ${list}; do
                filter_string="$(psql -d "${DBNAME}" -qtA \
                        --set replication_id="${line}" \
                        --set ON_ERROR_STOP=yes  <<EOF
SELECT
        r.src_filter
FROM   
        brigades.replications r
WHERE
        r.replication_id = :'replication_id'
EOF
)"

                split_filter "${line}"

        done
}

delete () {
        while [ "$#" -gt 0 ]; do
                case "$1" in
                        -f)
                                FORCE="x"
                                shift
                                ;;
                        *)
                                replication_uuid="$1"

                                break
                                ;;
                esac
        done

        if [ -z "${replication_uuid}" ]; then
                printdef "replication UUID not specified"
                exit 1
        fi

        echo "replication UUID: ${replication_uuid}"
        echo "Try to delete replication: ${replication_uuid}"

        if [ -z "${FORCE}" ]; then
                psql -d "${DBNAME}" -q \
                        --set replication_uuid="${replication_uuid}" \
                        --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

DELETE FROM 
        brigades.replicated_endpoints_ipv4 a
USING 
        brigades.replicated_endpoints_ipv4 e
        LEFT JOIN brigades.brigades b ON e.endpoint_ipv4 = b.endpoint_ipv4
WHERE
        e.replication_id = :'replication_uuid'
        AND a.src_dst = false
        AND a.endpoint_ipv4 = e.endpoint_ipv4
        AND b.endpoint_ipv4 IS NULL;

DELETE FROM
        brigades.replicated_endpoints_ipv4 a
WHERE
        a.replication_id = :'replication_uuid'
        AND a.src_dst = true;

DELETE FROM brigades.replications WHERE replication_id = :'replication_uuid';

COMMIT;
EOF
        else
                psql -d "${DBNAME}" -q \
                        --set brigades_schema="${BRIGADES_SCHEMA}" \
                        --set replication_uuid="${replication_uuid}" \
                        --set ON_ERROR_STOP=yes  <<EOF
BEGIN;

DELETE FROM 
        brigades.replicated_endpoints_ipv4 b
USING 
        brigades.replicated_endpoints_ipv4 e
WHERE
        e.replication_id = :'replication_uuid'
        AND b.endpoint_ipv4 = e.endpoint_ipv4;

DELETE FROM brigades.replications WHERE replication_id = :'replication_uuid';

COMMIT;
EOF
        fi

        echo "Deleted replication: ${replication_uuid}"

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
        autolist)
                auto_list "$@"
                ;;
        show)
                show "$@"
                ;;
        delete)
                delete "$@"
                ;;
        genmap)
                genmap "$@"
                ;;
        genconf)
                genconf "$@"
                ;;
        clear)
                clear "$@"
                ;;
        *)
                printdef "Unknown command: ${COMMAND}"
                exit 1
                ;;
esac


