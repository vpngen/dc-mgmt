#!/bin/sh

set -e

CONFDIR=${CONFDIR:-"${HOME}"}

STATS_SYNC_SERVER_ADDR=${STATS_SYNC_SERVER_ADDR:-$(cat "${CONFDIR}/statssyncserver")}
STATS_SYNC_SERVER_PORT=${STATS_SYNC_SERVER_PORT:-"22"}

SSH_KEY=${SSH_KEY:-"${CONFDIR}/.ssh/id_ed25519"}
DATADIR=${DATADIR:-"${CONFDIR}/vg-collectstats"}
REMOTE_DATADIR=${REMOTE_DATADIR:-"~/vg-collectstats"}

echo "[i] Sync file... ${STATS_SYNC_SERVER_ADDR}:${STATS_SYNC_SERVER_PORT}"

rsync \
        -e "ssh -o IdentitiesOnly=yes -o IdentityFile=${SSH_KEY} -o StrictHostKeyChecking=no -o ConnectTimeout=10 -p ${STATS_SYNC_SERVER_PORT}" \
        -avz \
        --remove-source-files \
        "${DATADIR}/" "${STATS_SYNC_SERVER_ADDR}:${REMOTE_DATADIR}/"
