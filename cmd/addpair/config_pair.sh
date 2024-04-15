#!/bin/sh

set +e

DBNAME=${DBNAME:-"vgrealm"}
SCHEMA=${SCHEMA:-"pairs"}

USERNAME=${USERNAME:-"ubuntu"}

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

printdef () {
        msg="$1"

        echo "ERROR: ${msg}" >&2
        echo "Usage: $0 [-e] [-f] [-net <control network>]" >&2
}

while [ "$#" -gt 0 ]; do
        case "$1" in
                -e)
                        ENABLE=true
                        shift
                        ;;
                -net)
                        CONTROL_NETWORK="$2"
                        shift 2
                        ;;
                -f)
                        FORCE=true
                        shift
                        ;;
                *)
                        printdef "Unknown option: $1"
                        ;;
        esac
done

if [ -z "${CONTROL_NETWORK}" ]; then
        CONTROL_NETWORK="0.0.0.0/0"
fi

echo "Control network: ${CONTROL_NETWORK}"
if [ -n "${ENABLE}" ]; then
        echo "Enable: true"
else
        echo "Enable: false"
fi

echo 

SQL_SELECT_NODES=$(cat <<EOF 
        SELECT 
                pair_id,
                control_ip
        FROM 
                :"schema_name".pairs
        WHERE 
                control_ip << :'control_ip'
EOF
)

if [ -z "${FORCE}" ]; then
        SQL_SELECT_NODES="${SQL_SELECT_NODES} AND (router_nacl_pubkey = '' OR ssh_ed25519_pubkey = '')"
fi

SQL_SELECT_NODES="${SQL_SELECT_NODES};"

#echo "SQL: ${SQL_SELECT_NODES}"

list=$(psql -d "${DBNAME}" \
        -q -X -t -A -F ";" \
        --set ON_ERROR_STOP=yes \
        --set schema_name="${SCHEMA}" \
        --set control_ip="${CONTROL_NETWORK}" <<EOF
${SQL_SELECT_NODES}
EOF
)

rc=$?
if [ $rc -ne 0 ]; then
        echo "[-]         Can't select: psql: ${rc}"
        exit 1
fi

cfgpair () {
        pair_id="$1"
        control_ip="$2"
        enable="$3"

        echo "    pair: ${pair_id}"
        echo "    control IP: ${control_ip}"
        if [ -n "${enable}" ]; then
                echo "    enable: true"
        else
                echo "    enable: false"
        fi

        CMD="cat /etc/ssh/ssh_host_ed25519_key.pub"
        ssh_ed25519_pubkey=$(ssh -o IdentitiesOnly=yes -o IdentityFile="${SSH_KEY}" -o StrictHostKeyChecking=no -T "${USERNAME}"@"${control_ip}" "${CMD}")
        rc=$?
        if [ $rc -ne 0 ]; then
                echo "[-]         Something wrong with ssh host key: $rc"
                
                return
        fi

        if [ -z "${ssh_ed25519_pubkey}" ]; then
                echo "[-]         Empty ssh host key"
                
                return
        fi

        echo "    ssh host key: ${ssh_ed25519_pubkey}"

        CMD="cat /etc/vg-router.json"
        router_nacl_pubkey=$(ssh -o IdentitiesOnly=yes -o IdentityFile="${SSH_KEY}" -o StrictHostKeyChecking=no -T "${USERNAME}"@"${control_ip}" "${CMD}")
        rc=$?
        if [ $rc -ne 0 ]; then
                echo "[-]         Something wrong with ssh router key: $rc"
                
                return
        fi

        if [ -z "${router_nacl_pubkey}" ]; then
                echo "[-]         Empty ssh router key"
                
                return
        fi

        echo "    router key: ${router_nacl_pubkey}"

        psql -d "${DBNAME}" \
                -q -X -t -A -F ";" \
                --set ON_ERROR_STOP=yes \
                --set schema_name="${SCHEMA}" \
                --set pair_id="${pair_id}" \
                --set control_ip="${control_ip}" \
                --set ssh_ed25519_pubkey="${ssh_ed25519_pubkey}" \
                --set router_nacl_pubkey="${router_nacl_pubkey}" <<EOF
        BEGIN;

        UPDATE :"schema_name".pairs
        SET router_nacl_pubkey = :'router_nacl_pubkey',
                ssh_ed25519_pubkey = :'ssh_ed25519_pubkey'
        WHERE pair_id = :'pair_id';

        COMMIT;
EOF

        rc=$?
        if [ $rc -ne 0 ]; then
                echo "[-]         Can't update: psql: ${rc}"
                
                return
        fi

        echo "Updated pair ${pair_id}"

        if [ -n "${enable}" ]; then
                psql -d "${DBNAME}" \
                        -q -X -t -A -F ";" \
                        --set ON_ERROR_STOP=yes \
                        --set schema_name="${SCHEMA}" \
                        --set pair_id="${pair_id}" <<EOF
                BEGIN;

                UPDATE :"schema_name".pairs
                SET is_active = true
                WHERE pair_id = :'pair_id';

                COMMIT;
EOF

                rc=$?
                if [ $rc -ne 0 ]; then
                        echo "[-]         Can't update: psql: ${rc}"
                
                        return
                fi
        fi
}

for node in ${list}; do
        pair_id=$(echo "${node}" | cut -d';' -f1)
        control_ip=$(echo "${node}" | cut -d';' -f2)

        echo "Pair: ${pair_id}"
        echo "Control IP: ${control_ip}"

        cfgpair "${pair_id}" "${control_ip}" "${DO_NOT_ENABLE}"
done