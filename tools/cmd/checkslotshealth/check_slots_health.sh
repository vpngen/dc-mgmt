#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}

PAIRS_SCHEMA=${PAIRS_SCHEMA:-"pairs"}
BRIGADES_SCHEMA=${BRIGADES_SCHEMA:-"brigades"}

BRIGADE_UUID=${BRIGADE_UUID:-"2817dabd-554c-4b39-842e-a65c4c2999f1"} # fake brigade uuid from purged brigades\
BRIGADE_ID="$(echo "${BRIGADE_UUID}" | xxd -r -p -l 16 | base32 | tr -d "=")"

if [ -x "$(dirname "$0")/gen" ]; then
        GEN_APP="$(dirname "$0")/gen"
elif [ -x "/opt/vg-dc-vpnapi/gen" ]; then
        GEN_APP="/opt/vg-dc-vpnapi/gen"
fi

printdef() {
        echo "Usage: [-f] [-inet <cidr>] [-enet <cidr>]"
        echo "       -f : force on not-active pairs"
        echo "       -r : recheck orphaned pairs"
        echo "       -inet : target control network filter"
        echo "       -enet : target external network filter"

        exit 1
}

orphan_slot () {
        control_ip=$1
        endpoint_ipv4=$2

        if [ -z "${control_ip}" ] || [ -z "${endpoint_ipv4}" ]; then
                echo "Missing control_ip or endpoint_ipv4" >&2
                return
        fi

        echo >&2
        echo "Orphaning slot: control_ip=${control_ip} endpoint_ipv4=${endpoint_ipv4}" >&2
        
        psql -d "${DBNAME}" -q -t -A \
                --set ON_ERROR_STOP=yes \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set endpoint_ipv4="${endpoint_ipv4}" <<EOF
                BEGIN;
                        INSERT INTO 
                                :"brigades_schema".orphaned_endpoints_ipv4
                                (endpoint_ipv4)
                        VALUES
                                (:'endpoint_ipv4')
                        ON CONFLICT DO NOTHING;
                COMMIT;
EOF
}

bless_slot () {
        control_ip=$1
        endpoint_ipv4=$2

        if [ -z "${control_ip}" ] || [ -z "${endpoint_ipv4}" ]; then
                echo "Missing control_ip or endpoint_ipv4" >&2
                return
        fi

        echo >&2
        echo "Blessing slot: control_ip=${control_ip} endpoint_ipv4=${endpoint_ipv4}" >&2
        
        psql -d "${DBNAME}" -q -t -A \
                --set ON_ERROR_STOP=yes \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set endpoint_ipv4="${endpoint_ipv4}" <<EOF
                BEGIN;
                        DELETE FROM 
                                :"brigades_schema".orphaned_endpoints_ipv4
                        WHERE
                               endpoint_ipv4=:'endpoint_ipv4';
                COMMIT;
EOF
}

while [ $# -gt 0 ]; do
        case "$1" in
                -inet)
                        INET_FILTER="$2"
                        shift
                        ;;
                -enet)
                        ENET_FILTER="$2"
                        shift
                        ;;
                -control_ip)
                        CONTROL_IP="$2"
                        shift
                        ;;
                -endpoint_ipv4)
                        ENDPOINT_IPV4="$2"
                        shift
                        ;;
                -id)
                        BRIGADE_ID="$2"
                        shift
                        ;;
                -name)
                        GEN_NAME="$2"
                        shift
                        ;;
                -person)
                        GEN_PERSON="$2"
                        shift
                        ;;
                -desc)
                        GEN_DESC="$2"
                        shift
                        ;;
                -url)
                        GEN_URL="$2"
                        shift
                        ;;
                -f)
                        FORCE=yes
                        ;;
                -r)
                        RECHECK=yes
                        ;;
                *)
                        printdef "Unknown option: $1"
                        ;;
        esac
        shift
done

if [ -z "$CONTROL_IP" ]; then

        if [ -z "$INET_FILTER" ]; then
                INET_FILTER="0.0.0.0/0"
        fi

        if [ -z "$ENET_FILTER" ]; then
                ENET_FILTER="0.0.0.0/0"
        fi

        IS_ACTIVE="p.is_active = true"
        if [ "${FORCE}" = "yes" ]; then
                IS_ACTIVE="true"
        fi

        IS_RECHECK="o.endpoint_ipv4 IS NULL"
        if [ -n "${RECHECK}" ]; then
                IS_RECHECK="o.endpoint_ipv4 IS NOT NULL"
        fi

        count=$(psql -d "${DBNAME}" -q -t -A \
                --set ON_ERROR_STOP=yes \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set pairs_schema="${PAIRS_SCHEMA}" \
                --set inet_filter="${INET_FILTER}" \
                --set enet_filter="${ENET_FILTER}" <<EOF
SELECT
        count(*)
FROM 
        :"pairs_schema".pairs AS p
        JOIN :"pairs_schema".pairs_endpoints_ipv4 AS pei ON pei.pair_id=p.pair_id
        LEFT JOIN :"brigades_schema".orphaned_endpoints_ipv4 AS o ON o.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"brigades_schema".reserved_endpoints_ipv4 AS r ON r.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"brigades_schema".brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
WHERE
        ${IS_ACTIVE}
        AND ${IS_RECHECK}
        AND r.endpoint_ipv4 IS NULL
        AND b.endpoint_ipv4 IS NULL
        AND p.control_ip <<= :'inet_filter' 
        AND pei.endpoint_ipv4 <<= :'enet_filter';
EOF
)

        date +"%Y-%m-%dT%H:%M:%S" >&2
        echo "Control network: ${INET_FILTER}" >&2
        echo "External network: ${ENET_FILTER}" >&2
        echo "Slots found: ${count}" >&2

        slots=$(psql -d "${DBNAME}" -q -t -A \
                --set ON_ERROR_STOP=yes \
                --set brigades_schema="${BRIGADES_SCHEMA}" \
                --set pairs_schema="${PAIRS_SCHEMA}" \
                --set inet_filter="${INET_FILTER}" \
                --set enet_filter="${ENET_FILTER}" <<EOF
SELECT
        p.control_ip,
        pei.endpoint_ipv4
FROM 
        :"pairs_schema".pairs AS p
        JOIN :"pairs_schema".pairs_endpoints_ipv4 AS pei ON pei.pair_id=p.pair_id
        LEFT JOIN :"brigades_schema".orphaned_endpoints_ipv4 AS o ON o.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"brigades_schema".reserved_endpoints_ipv4 AS r ON r.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"brigades_schema".brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
WHERE
        ${IS_ACTIVE}
        AND ${IS_RECHECK}
        AND r.endpoint_ipv4 IS NULL
        AND b.endpoint_ipv4 IS NULL
        AND p.control_ip <<= :'inet_filter' 
        AND pei.endpoint_ipv4 <<= :'enet_filter';
EOF
)

        if [ -z "$slots" ]; then
                echo "No slots found" >&2
                exit 0
        fi

        GENERATED="$(${GEN_APP})"
        GEN_NAME=$(echo "$GENERATED" | cut -d ' ' -f 2)
        GEN_PERSON=$(echo "$GENERATED" | cut -d ' ' -f 3)
        GEN_DESC=$(echo "$GENERATED" | cut -d ' ' -f 4)
        GEN_URL=$(echo "$GENERATED" | cut -d ' ' -f 5)

        echo "Brigade name: $(echo "$GEN_NAME"| base64 -d)" >&2

        for slot in $slots; do
                control_ip=$(echo "$slot" | cut -d '|' -f 1)
                endpoint_ipv4=$(echo "$slot" | cut -d '|' -f 2)
                
                if [ -n "${RECHECK}" ]; then
                        BLESS="-r"
                fi

                flock -x -E 1 -w 60 /tmp/modbrigade.lock "$0" \
                        -control_ip "${control_ip}" -endpoint_ipv4 "${endpoint_ipv4}" \
                        -id "${BRIGADE_ID}" \
                        -name "${GEN_NAME}" -person "${GEN_PERSON}" -desc "${GEN_DESC}" -url "${GEN_URL}" \
                        "${BLESS}"

                sleep 1
        done

        echo "Finish: $(date +"%Y-%m-%dT%H:%M:%S")" >&2

        exit 0
else
        if [ -z "${ENDPOINT_IPV4}" ]; then
                echo "Missing endpoint_ipv4" >&2
                exit 1
        fi

        if [ -z "${CONTROL_IP}" ]; then
                echo "Missing control_ip" >&2
                exit 1
        fi

        if [ -z "${BRIGADE_ID}" ]; then
                echo "Missing brigade_id" >&2
                exit 1
        fi

        if [ -z "${GEN_NAME}" ] || [ -z "${GEN_PERSON}" ] || [ -z "${GEN_DESC}" ] || [ -z "${GEN_URL}" ]; then
                echo "Missing brigade name, person, desc or url" >&2
                exit 1
        fi

        USERNAME=${USERNAME:-"_serega_"}

        
        if [ -z "${SSH_KEY}" ]; then
                if [ -s "${HOME}/.ssh/id_ed25519" ]; then
                        SSH_KEY="${HOME}/.ssh/id_ed25519"
                elif [ -s "${HOME}/.ssh/id_ecdsa" ]; then
                        SSH_KEY="${HOME}/.ssh/id_ecdsa"
                else
                        echo "[-]         SSH key not found"
                        exit 1
                fi
        fi

        VPN_INT_NET4=${VPN_INT_NET4:-"100.64.42.0/24"}
        VPN_INT_NET6=${VPN_INT_NET6:-"fdee:64:42::/64"}
        VPN_DNS4=${VPN_DNS4:-"100.64.42.1"}
        VPN_DNS6=${VPN_DNS6:-"fdee:64:42::1"}
        VPN_KEYDESK_IPV6=${VPN_KEYDESK_IPV6:-"fdaa:64:42::777"}
        DOMAIN_NAME=${DOMAIN_NAME:-"bodywolfdiarylyric.org"}

        echo >&2

        set +e

        echo "Slot: control_ip=${CONTROL_IP} endpoint_ipv4=${ENDPOINT_IPV4}" >&2
        #echo "Creating brigade: ${ARGS}"

        echo "$(date +"%Y-%m-%dT%H:%M:%S"):... creating brigade: brigade_id=${BRIGADE_ID} brigade name=$(echo "$GEN_NAME"| base64 -d)" >&2        
        echo >&2
        
        #shellcheck disable=SC2034
        CONFIG=$(ssh -o IdentitiesOnly=yes -o IdentityFile="${SSH_KEY}" -o StrictHostKeyChecking=no "${USERNAME}"@"${CONTROL_IP}" \
        "create" -ch -j -id "${BRIGADE_ID}" \
        -ep4 "${ENDPOINT_IPV4}" \
        -name "${GEN_NAME}" -person "${GEN_PERSON}" -desc "${GEN_DESC}" -url "${GEN_URL}" \
        -int4 "${VPN_INT_NET4}" -int6 "${VPN_INT_NET6}" -dns4 "${VPN_DNS4}" -dns6 "${VPN_DNS6}" -kd6 "${VPN_KEYDESK_IPV6}" \
        -dn "${DOMAIN_NAME}" \
        -wg native -ovc amnezia -outline access_key
        )
        rc=$?
        if [ $rc -ne 0 ]; then
                echo "$(date +"%Y-%m-%dT%H:%M:%S"): SSH error code: $rc" >&2

                # orphan slot
                orphan_slot "${CONTROL_IP}" "${ENDPOINT_IPV4}"

                ORPHANED=yes
        fi

        echo >&2
        echo "$(date +"%Y-%m-%dT%H:%M:%S"):... destroying brigade: brigade_id=${BRIGADE_ID}" >&2
        echo >&2

        ssh -o IdentitiesOnly=yes -o IdentityFile="${SSH_KEY}" -o StrictHostKeyChecking=no "${USERNAME}"@"${CONTROL_IP}" \
        "destroy" -id "${BRIGADE_ID}" 
        rc=$?
        if [ $rc -ne 0 ]; then
                echo "$(date +"%Y-%m-%dT%H:%M:%S"): SSH error code: $rc" >&2
  
                if [ -z "${ORPHANED}" ]; then
                        # orphan slot
                        orphan_slot "${CONTROL_IP}" "${ENDPOINT_IPV4}"
                        
                        ORPHANED=yes
                fi
        fi

        if [ -z "${ORPHANED}" ]; then
                echo "[+] Slot ${ENDPOINT_IPV4} is clear" >&2

                if [ -n "${RECHECK}" ]; then
                        # bless slot
                        bless_slot "${CONTROL_IP}" "${ENDPOINT_IPV4}"
                fi
        else 
                echo "[-] Slot ${ENDPOINT_IPV4} was orphaned" >&2
        fi

        echo >&2
fi