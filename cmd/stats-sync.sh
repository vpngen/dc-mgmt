#!/bin/sh

CONFDIR=${CONFDIR:-"${HOME}"}

STATS_SYNC_SERVER_ADDR=${STATS_SYNC_SERVER_ADDR:-$(cat "${CONFDIR}/statssyncserver")}
STATS_SYNC_SERVER_PORT=${STATS_SYNC_SERVER_PORT:-"22"}
STATS_SYNC_SERVER_JUMPS=${STATS_SYNC_SERVER_JUMPS:-""} # separated by commas

SSH_KEY=${SSH_KEY:-"${CONFDIR}/.ssh/id_ed25519"}
DATADIR=${DATADIR:-"${CONFDIR}/vg-collectstats"}

# Use rrsync on remote server
#REMOTE_DATADIR=${REMOTE_DATADIR:-"~/vg-collectstats"}

jumps=""
if [ -n "${STATS_SYNC_SERVER_JUMPS}" ]; then
        jumps="-J ${STATS_SYNC_SERVER_JUMPS}"
fi

echo "[i] Sync file... ${STATS_SYNC_SERVER_ADDR}:${STATS_SYNC_SERVER_PORT}"
if [ -n "${STATS_SYNC_SERVER_JUMPS}" ]; then
        echo "[i] Jumps: ${STATS_SYNC_SERVER_JUMPS}"
fi

rsync \
        -e "ssh -o IdentitiesOnly=yes -o IdentityFile=${SSH_KEY} -o StrictHostKeyChecking=no -o ConnectTimeout=10 -p ${STATS_SYNC_SERVER_PORT} ${jumps}" \
        -avz \
        --remove-source-files \
        "${DATADIR}/" "${STATS_SYNC_SERVER_ADDR}:/"

if [ "$?" -ne 0 ]; then
        echo "[-] Can't rsync"
        exit 0
fi
