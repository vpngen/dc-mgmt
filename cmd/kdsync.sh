#!/bin/sh

ETC="/etc/vgrealm"
DBNAME=${DBNAME:-$(cat "${ETC}/dbname")}
SCHEMA=${SCHEMA:-$(cat "${ETC}/schema")}
SSHKEY=${SSHKEY:-"${ETC}/id_ecdsa"}
KDSYNCSERVER=${KDSYNCSERVER:-$(cat "${ETC}/kdsyncserver")}

echo "[i] Fetch pairs...."

list=$(psql -d ${DBNAME} -v ON_ERROR_STOP=yes -t -A -F ";" --set schema_name="${SCHEMA}" <<EOF 
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

echo "[i] Sync file... ${KDSYNCSERVER}"

cmd="cat > vpn-works-wolfs.csv.tmp && mv -f vpn-works-wolfs.csv.tmp vpn-works-wolfs.csv && touch vpn-works-keydesks.reload"
set +x
echo "${list}" | ssh -o IdentitiesOnly=yes -o IdentityFile=${SSHKEY} -o StrictHostKeyChecking=no -T ${KDSYNCSERVER} ${cmd}
rc=$?
if [ $rc -ne 0 ]; then
	echo "[-] Can't ssh: $rc"
        exit 0
fi

echo "[i] Finish"

exit 0
