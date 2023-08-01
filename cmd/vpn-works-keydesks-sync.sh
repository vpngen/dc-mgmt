#!/bin/sh

CONFDIR=${CONFDIR:-"${HOME}"}

DBNAME=${DBNAME:-"vgrealm"}
SCHEMA=${SCHEMA:-"brigades"}
SSH_KEY=${SSH_KEY:-"${CONFDIR}/.ssh/id_ed25519"}

if [ -s "/etc/vg-dc-vpnapi/vpn-works-keydesks-sync.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-vpnapi/vpn-works-keydesks-sync.env"
fi

if [ -s  "/etc/vg-dc-mgmt/dc-name.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-mgmt/dc-name.env"
fi

VPN_WORKS_KEYDESKS_SERVER_ADDR=${VPN_WORKS_KEYDESKS_SERVER_ADDR:-$(cat "${CONFDIR}/kdsyncserver")}
VPN_WORKS_KEYDESKS_SERVER_PORT=${VPN_WORKS_KEYDESKS_SERVER_PORT:-"22"}
VPN_WORKS_KEYDESKS_SERVER_JUMPS=${VPN_WORKS_KEYDESKS_SERVER_JUMPS:-""} # separated by commas

DC_NAME=${DC_NAME:-"unknown"}

echo "[i] Fetch pairs...."

list=$(psql -d "${DBNAME}" \
        -q -X -t -A -F ";" \
        --set ON_ERROR_STOP=yes \
        --set schema_name="${SCHEMA}" <<EOF 
	SELECT 
		endpoint_ipv4,
		keydesk_ipv6 
	FROM 
		:"schema_name".brigades
EOF
)
rc=$?
if [ $rc -ne 0 ]; then
	echo "[-] Can't select: psql: ${rc}"
	exit 0
fi

jumps=""
if [ -n "${VPN_WORKS_KEYDESKS_SERVER_JUMPS}" ]; then
        jumps=" -J ${VPN_WORKS_KEYDESKS_SERVER_JUMPS}"
fi

echo "[i] Sync file... ${VPN_WORKS_KEYDESKS_SERVER_ADDR}:${VPN_WORKS_KEYDESKS_SERVER_PORT}"
if [ -n "${VPN_WORKS_KEYDESKS_SERVER_JUMPS}" ]; then
        echo "[i] Jumps: ${VPN_WORKS_KEYDESKS_SERVER_JUMPS}"
fi

CSV_FILENAME="vpn-works-${DC_NAME}.csv"
CSV_FILENAME_TMP="${CSV_FILENAME}.tmp"

cmd="cat > ${CSV_FILENAME_TMP} && mv -f ${CSV_FILENAME_TMP} ${CSV_FILENAME} && touch vpn-works-keydesks.reload"
set +x
echo "${list}" | ssh -o IdentitiesOnly=yes \
                -o IdentityFile="${SSH_KEY}" \
                -o StrictHostKeyChecking=no \
                -o ConnectTimeout=10 \
                -T "${VPN_WORKS_KEYDESKS_SERVER_ADDR}" \
                -p "${VPN_WORKS_KEYDESKS_SERVER_PORT}" \
                ${jumps} \
                "${cmd}"
rc=$?
if [ $rc -ne 0 ]; then
	echo "[-] Can't ssh: $rc"
        exit 0
fi

echo "[i] Finish"

exit 0
