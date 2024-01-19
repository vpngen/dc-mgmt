#!/bin/sh

ADMIN_USER="vgadmin"
VPNAPI_USER="vgvpnapi"
STATS_USER="vgstats"
SNAPSHOTS_USER="vgsnaps"
MIGRATIONS_USER="vgmigr"

SNAPSHOT_SHARE_GROUP="vgsnaps"

create_users () {
        if id "${ADMIN_USER}" >/dev/null 2>&1; then
                echo "user ${ADMIN_USER} already exists"
        else
                useradd -p "*" -m "${ADMIN_USER}" -s /bin/bash
        fi

        if id "${VPNAPI_USER}" >/dev/null 2>&1; then
                echo "user ${VPNAPI_USER} already exists"
        else
                useradd -p "*" -m "${VPNAPI_USER}" -s /bin/bash
        fi

        if id "${STATS_USER}" >/dev/null 2>&1; then
                echo "user ${STATS_USER} already exists"
        else
                useradd -p "*" -m "${STATS_USER}" -s /bin/bash
        fi

        if id "${SNAPSHOTS_USER}" >/dev/null 2>&1; then
                echo "user ${SNAPSHOTS_USER} already exists"
        else
                useradd -p "*" -m "${SNAPSHOTS_USER}" -s /bin/bash
        fi

        if id "${MIGRATIONS_USER}" >/dev/null 2>&1; then
                echo "user ${MIGRATIONS_USER} already exists"
        else
                useradd -p "*" -m "${MIGRATIONS_USER}" -s /bin/bash -G "${SNAPSHOT_SHARE_GROUP}"
        fi
}

cleanInstall() {
	printf "Pre Install of an clean install\n"

        set -e

        # Create new users
        create_users


}

upgrade() {
    	printf "Pre Install of an upgrade\n"

        # Create new users
        create_users

        systemctl stop vg-dc-stats.timer ||:
}

# Step 2, check if this is a clean install or an upgrade
action="$1"
if  [ "$1" = "configure" ] && [ -z "$2" ]; then
 	# Alpine linux does not pass args, and deb passes $1=configure
 	action="install"
elif [ "$1" = "configure" ] && [ -n "$2" ]; then
   	# deb passes $1=configure $2=<current version>
	action="upgrade"
fi

case "$action" in
  "1" | "install")
    printf "\033[31m install... \033[0m\n"
    cleanInstall
    ;;
  "2" | "upgrade")
    printf "\033[31m upgrade... \033[0m\n"
    upgrade
    ;;
  *)
    # $1 == version being installed
    printf "\033[31m default... \033[0m\n"
    cleanInstall
    ;;
esac


