#!/bin/bash

set -e

# Script varible set
PREINSTALL_SCRIPT_NAME="$0"
PROJECT_NAME="realm-admin"
TMP_DIR="temp_dir"

export INSTALLATION_TEMP_DIR="${PROJECT_NAME}_${TMP_DIR}"


DBNAME=${DBNAME:-"vgralm"}
PAIRS_DBUSER=${PAIRS_DBUSER:-"vgrealm"}
BRIGADES_DBUSER=${BRIGADES_DBUSER:-"vgrealm"}
PAIRS_SCHEMA=${PSCHEMA:-"pairs"}
BRIGADES_SCHEMA=${BSCHEMA:-"brigades"}

REALM_ADMIN=${REALM_ADMIN:-"vgrealm"}
SSH_API_USER=${SSH_API_USER:="_valera_"}


function doCheckDBRunAndStart {
    sqlPID=$(pgrep postgresql)

    if [ -n "$sqlPID" ]; then
	return 0
    fi
    return 1
}

function doInstallUsers {
    echo "doInstallUsers"
}

function doInitDB {
    echo "doInitDB"
    mkdir -p /tmp/${INSTALLATION_TEMP_DIR}/sql
}

function mainFunc {
    if ! doCheckDBRunAndStart; then
	echo "Starting PostgreSQL..."
	#service postgresql start
    fi

    doInstallUsers
    doInitDB
}

case $1 in
    *)
	mainFunc
esac
