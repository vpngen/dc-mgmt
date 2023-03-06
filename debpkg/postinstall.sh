#!/bin/bash

PROJECT_NAME="realm-admin"
TMP_DIR="temp_dir"

INSTALLATION_TEMP_DIR="${PROJECT_NAME}_${TMP_DIR}"

DBNAME=${DBNAME:-"vgrealm"}
PAIRS_DBUSER=${PAIRS_DBUSER:-"vgrealm"}
BRIGADES_DBUSER=${BRIGADES_DBUSER:-"vgrealm"}
SCHEMA_PAIRS=${PSCHEMA:-"pairs"}
SCHEMA_BRIGADES=${BSCHEMA:-"brigades"}

function doCheckDBExist {
    db=$(sudo -i -u postgres psql -l | grep "${DBNAME}")
    if [ -n "$db" ]; then 
	return 0
    fi
    return 1
}

function doInitDB {
    echo " [=] Starting init DB..."
    SQL_FILES_PATH="/tmp/${INSTALLATION_TEMP_DIR}/sql"

    if ! doCheckDBExist; then 
	sudo -i -u postgres psql -c "CREATE DATABASE ${DBNAME};"
    fi

    sudo -i -u postgres psql -v -d "${DBNAME}" \
	--set schema_pairs_name="${SCHEMA_PAIRS}" \
	--set schema_brigades_name="${SCHEMA_BRIGADES}" \
	--set pairs_dbuser="${PAIRS_DBUSER}" \
	--set brigades_dbuser="${BRIGADES_DBUSER}" \
	< "$SQL_FILES_PATH"/000-install.sql
    
    echo " [=]  Init DB finished"

}

function doRemoveTempDir {
    echo " [=] Remove Temp dir: ${INSTALLATION_TEMP_DIR}"

    rm -rf "/tmp/${INSTALLATION_TEMP_DIR}"
}

function doRemoveDebFile {
    echo " [=] Remove Deb-file"

    find ./ -type f -name "*.deb" -exec rm -f {} \;
}

doInitDB
doRemoveTempDir
doRemoveDebFile
