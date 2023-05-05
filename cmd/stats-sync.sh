#!/bin/sh

set -e

STATSSYNCSERVER=${STATSSYNCSERVER:-$(cat "${HOME}/statssyncserver")}
SSHKEY=${SSHKEY:-"${HOME}/.ssh/id_ed25519"}
DATADIR=${DATADIR:-"${HOME}/vg-collectstats"}
REMOTEDATADIR=${REMOTEDATADIR:-"~/vg-collectstats"}

echo "[i] Sync file... ${STATSSYNCSERVER}"

rsync -e "ssh -o IdentitiesOnly=yes -o IdentityFile=${SSHKEY} -o StrictHostKeyChecking=no" -avz --remove-source-files "${DATADIR}/" "${STATSSYNCSERVER}:${REMOTEDATADIR}/"
