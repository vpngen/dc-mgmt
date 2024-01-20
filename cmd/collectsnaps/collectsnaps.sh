#!/bin/sh

printdef() {
    echo "Usage: -tag <tag> [-ad] [-r] [-mnt <maintenance mode till unixtime>] [-net <cidr>]"
    exit 1
}

if [ $# -eq 0 ]; then 
    printdef
fi

basedir=$(dirname "$0")

if [ -s  "/etc/vg-dc-mgmt/dc-name.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-mgmt/dc-name.env"
fi

if [ -s "/etc/vg-dc-mgmt/realmfp.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-mgmt/realmfp.env"
fi

if [ -s "/etc/vg-dc-snaps/collectsnaps.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-snaps/collectsnaps.env"
fi

DB_URL="${DB_URL}" \
DC_ID="${DC_ID}" \
DC_NAME="${DC_NAME}" \
SSH_KEY="${SSH_KEY}" \
REALM_FP="${REALM_FP}" \
REALMS_KEYS_PATH="${REALMS_KEYS_PATH}" \
SNAPSHOTS_BASE_DIR="${SNAPSHOTS_BASE_DIR}" \
flock -x -n /tmp/collectsnaps.lock "${basedir}"/collectsnaps "$@"
