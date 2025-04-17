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

EXTRA_ARGS=""

if [ -n "${KEEP_LAST}" ]; then
        EXTRA_ARGS="${EXTRA_ARGS} -keep_last ${KEEP_LAST}"
fi

if [ -n "${KEEP_WITHIN}" ]; then
        EXTRA_ARGS="${EXTRA_ARGS} -keep_within ${KEEP_WITHIN}"
fi

if [ -n "${KEEP_HOURLY}" ]; then
        EXTRA_ARGS="${EXTRA_ARGS} -keep_hourly ${KEEP_HOURLY}"
fi

if [ -n "${KEEP_DAILY}" ]; then
        EXTRA_ARGS="${EXTRA_ARGS} -keep_daily ${KEEP_DAILY}"
fi

if [ -n "${KEEP_WEEKLY}" ]; then
        EXTRA_ARGS="${EXTRA_ARGS} -keep_weekly ${KEEP_WEEKLY}"
fi

if [ -n "${KEEP_MONTHLY}" ]; then
        EXTRA_ARGS="${EXTRA_ARGS} -keep_monthly ${KEEP_MONTHLY}"
fi

if [ -n "${KEEP_YEARLY}" ]; then
        EXTRA_ARGS="${EXTRA_ARGS} -keep_yearly ${KEEP_YEARLY}"
fi

DB_URL="${DB_URL}" \
DC_ID="${DC_ID}" \
DC_NAME="${DC_NAME}" \
SSH_KEY="${SSH_KEY}" \
REALM_FP="${REALM_FP}" \
REALMS_KEYS_PATH="${REALMS_KEYS_PATH}" \
SNAPSHOTS_BASE_DIR="${SNAPSHOTS_BASE_DIR}" \
"${basedir}"/collectsnaps "$@" ${EXTRA_ARGS}
