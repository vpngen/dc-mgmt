#!/bin/bash

PROJECT_NAME="realm-admin"
TMP_DIR="temp_dir"

INSTALLATION_TEMP_DIR="${PROJECT_NAME}_${TMP_DIR}"

DBNAME=${DBNAME:-"vgrealm"}
PAIRS_DBUSER=${PAIRS_DBUSER:-"vgrealm"}
BRIGADES_DBUSER=${BRIGADES_DBUSER:-"vgrealm"}
SCHEMA_PAIRS=${PSCHEMA:-"pairs"}
SCHEMA_BRIGADES=${BSCHEMA:-"brigades"}
SCHEMA_STATS=${STSCHEMA:-"stats"}
STATS_DBUSER=${STATS_DBUSER:-"vgrealm"}
MINISTRY_STATS_DBUSER=${MNST_STATS_DBUSER:-"vgrealm"}

function doCheckDBExist {
    db=$(sudo -i -u postgres psql -l | grep "${DBNAME}")
    if [ -n "$db" ]; then 
	echo " [!] DB ${DBNAME} is exist"
	return 0
    fi
    echo " [!] DB ${DBNAME} is not exist"
    return 1
}

function doInitDB {
    echo " [=] Starting init DB..."
    SQL_FILES_PATH="/tmp/${INSTALLATION_TEMP_DIR}/sql"

    if ! doCheckDBExist; then 
	echo " [+] Create DB ${DBNAME}"
	sudo -i -u postgres psql -c "CREATE DATABASE ${DBNAME};"
    fi

    sudo -i -u postgres cat ${SQL_FILES_PATH}/000-install.sql | sudo -i -u postgres psql \
	-v -d "${DBNAME}" \
	--set schema_pairs_name="${SCHEMA_PAIRS}" \
	--set schema_brigades_name="${SCHEMA_BRIGADES}" \
	--set pairs_dbuser="${PAIRS_DBUSER}" \
	--set brigades_dbuser="${BRIGADES_DBUSER}" \
	--set schema_stats_name="${SCHEMA_STATS}" \
	--set stats_dbuser="${STATS_DBUSER}" \
	--set ministry_stats_dbuser="${MINISTRY_STATS_DBUSER}" 
    
    echo " [=]  Init DB finished"

}

function doRemoveTempDir {
    echo " [=] Remove Temp dir: ${INSTALLATION_TEMP_DIR}"
    rm -rf "/tmp/${INSTALLATION_TEMP_DIR}"
}

# dpkg Error
function doRemoveDebFile {
    echo " [=] Remove Deb-file"
    files=$(find ./ -maxdepth 2 -type f -regextype posix-extended -regex ".*${PROJECT_NAME}.*\.deb$" -exec rm -f {} \;)
    echo "$files"
}

doInitDB
doRemoveTempDir
doRemoveDebFile

echo "[=] Finished install ${PROJECT_NAME}"
