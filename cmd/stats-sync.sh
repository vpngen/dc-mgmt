#!/bin/sh

CONFDIR=${CONFDIR:-"${HOME}"}

STATS_SYNC_SERVER_ADDR=${STATS_SYNC_SERVER_ADDR:-$(cat "${CONFDIR}/statssyncserver")}
STATS_SYNC_SERVER_PORT=${STATS_SYNC_SERVER_PORT:-"22"}
STATS_SYNC_SERVER_JUMPS=${STATS_SYNC_SERVER_JUMPS:-""} # separated by commas

SSH_KEY=${SSH_KEY:-"${CONFDIR}/.ssh/id_ed25519"}
DATADIR=${DATADIR:-"${CONFDIR}/vg-collectstats"}
REMOTE_DATADIR=${REMOTE_DATADIR:-"~/vg-collectstats"}

jumps=""
for jump_hosts in $(echo "${STATS_SYNC_SERVER_JUMPS}" | tr "," "\n"); do
        jumps="${jumps} -J ${jump_hosts}"
done

echo "[i] Sync file... ${STATS_SYNC_SERVER_ADDR}:${STATS_SYNC_SERVER_PORT}"
if [ -n "${jumps}" ]; then
        echo "[i] Jumps: ${jumps}"
fi

rsync \
        -e "ssh -o IdentitiesOnly=yes -o IdentityFile=${SSH_KEY} -o StrictHostKeyChecking=no -o ConnectTimeout=10 -p ${STATS_SYNC_SERVER_PORT} ${jumps}" \
        -avz \
        --remove-source-files \
        "${DATADIR}/" "${STATS_SYNC_SERVER_ADDR}:${REMOTE_DATADIR}/"

if [ "$?" -ne 0 ]; then
        echo "[-] Can't rsync"
        exit 0
fi
