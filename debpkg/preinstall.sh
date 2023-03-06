#!/bin/bash

set -e

# Script varible set
PROJECT_NAME="realm-admin"
TMP_DIR="temp_dir"

INSTALLATION_TEMP_DIR="${PROJECT_NAME}_${TMP_DIR}"


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

function doMakeDBInitDir {
    echo " [=] Make DB Init Temp Dir"
    mkdir -p /tmp/${INSTALLATION_TEMP_DIR}/sql
}

function mainFunc {
    if ! doCheckDBRunAndStart; then
	echo " [=] Starting PostgreSQL..."
	service postgresql start
    fi

    doInstallUsers
    doMakeDBInitDir
}

case $1 in
    *)
	mainFunc
esac
