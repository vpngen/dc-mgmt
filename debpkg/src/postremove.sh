#!/bin/sh

ADMIN_USER="vgadmin"
VPNAPI_USER="vgvpnapi"
STATS_USER="vgstats"
SNAPSHOTS_USER="vgsnaps"
MIGRATIONS_USER="vgmigr"

remove_users () {
        if id "${ADMIN_USER}" >/dev/null 2>&1; then
                userdel -r "${ADMIN_USER}"
        else
                echo "user ${ADMIN_USER} does not exists"
        fi

        if id "${VPNAPI_USER}" >/dev/null 2>&1; then
                userdel -r "${VPNAPI_USER}"
        else
                echo "user ${VPNAPI_USER} does not exists"
        fi

        if id "${STATS_USER}" >/dev/null 2>&1; then
                userdel -r "${STATS_USER}"
        else
                echo "user ${STATS_USER} does not exists"
        fi

        if id "${SNAPSHOTS_USER}" >/dev/null 2>&1; then
                userdel -r "${SNAPSHOTS_USER}"
        else
                echo "user ${SNAPSHOTS_USER} does not exists"
        fi

        if id "${MIGRATIONS_USER}" >/dev/null 2>&1; then
                userdel -r "${MIGRATIONS_USER}"
        else
                echo "user ${MIGRATIONS_USER} does not exists"
        fi
}

remove() {
        printf "Post Remove of a normal remove\n"

        remove_users

        printf "Reload the service unit from disk\n"
        systemctl daemon-reload ||:
}

purge() {
    printf "\033[32m Pre Remove purge, deb only\033[0m\n"
}

upgrade() {
    printf "\033[32m Pre Remove of an upgrade\033[0m\n"
}

echo "$@"

action="$1"

case "$action" in
  "0" | "remove")
    remove
    ;;
  "1" | "upgrade")
    upgrade
    ;;
  "purge")
    purge
    ;;
  *)
    printf "\033[32m Alpine\033[0m"
    remove
    ;;
esac
