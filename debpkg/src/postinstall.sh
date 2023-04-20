#!/bin/sh

DBUSER=${DBUSER:-"postgres"}
DBNAME=${DBNAME:-"vgrealm"}
SCHEMA_PAIRS=${SCHEMA_PAIRS:-"pairs"}
PAIRS_DBUSER=${PAIRS_DBUSER:-"vgadmin"}
SCHEMA_BRIGADES=${SCHEMA_BRIGADES:-"brigades"}
BRIGADES_DBUSER=${BRIGADES_DBUSER:-"vgvpnapi"}
SCHEMA_STATS=${SCHEMA_STATS:-"stats"}
STATS_DBUSER=${STATS_DBUSER:-"vgstats"}

SQL_DIR="/usr/share/vg-dc-stats"

load_sql_file () {
        cat "$1" | sudo -u "${DBUSER}" psql -d "${DBNAME}" -v ON_ERROR_STOP=yes \
                --set schema_pairs_name="${SCHEMA_PAIRS}" \
                --set pairs_dbuser="${PAIRS_DBUSER}" \
                --set schema_brigades_name="${SCHEMA_BRIGADES}" \
                --set brigades_dbuser="${BRIGADES_DBUSER}" \
                --set schema_stats_name="${SCHEMA_STATS}" \
                --set stats_dbuser="${STATS_DBUSER}" 
        rc=$?
        if [ ${rc} -ne 0 ]; then
                exit 1
        fi
}

init_database () {
        # Create database
        sudo -u "${DBUSER}" psql --set dbname="${DBNAME}" -v ON_ERROR_STOP=yes -c "CREATE DATABASE :dbname;"
        rc=$?
        if [ ${rc} -ne 0 ]; then
                exit 1
        fi

        # Init database

        load_sql_file "${SQL_DIR}/init/000-versioning.sql"
        load_sql_file "${SQL_DIR}/init/001-init.sql"

}

apply_database_patches () {
        for patch in ls "${SQL_DIR}/patches/"*.sql; do
                load_sql_file "${patch}"
        done

        sudo -u "${DBUSER}" psql -v ON_ERROR_STOP=yes -c "SELECT pg_reload_conf();"
        rc=$?
        if [ ${rc} -ne 0 ]; then
                exit 1
        fi
}


cleanInstall() {
	printf "Post Install of an clean install\n"

        set -e

        init_database
        apply_database_patches

    	printf "Reload the service unit from disk\n"
    	systemctl daemon-reload ||:
	systemctl start vg-dc-stats.timer ||:
}

upgrade() {
    	printf "Post Install of an upgrade\n"

        apply_database_patches

    	printf "Reload the service unit from disk\n"
    	systemctl daemon-reload ||:
	systemctl restart vg-dc-stats.timer ||:
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
    cleanInstall
    ;;
  "2" | "upgrade")
    printf "\033[32m Post Install of an upgrade\033[0m\n"
    upgrade
    ;;
  *)
    # $1 == version being installed
    printf "\033[32m Alpine\033[0m"
    cleanInstall
    ;;
esac


