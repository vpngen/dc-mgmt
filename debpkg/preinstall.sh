#!/bin/bash

set -e

# Script varible set
PROJECT_NAME="realm-admin"
TMP_DIR="temp_dir"

INSTALLATION_TEMP_DIR="${PROJECT_NAME}_${TMP_DIR}"

REALM_ADMIN=${REALM_ADMIN:-"vgrealm"}
SSH_API_USER=${SSH_API_USER:="_valera_"}


function doCheckDBRunAndStart {
    dbServerStatus=$(service postgresql status | grep online)

    if [ -n "$dbServerStatus" ]; then
	echo " [!] Proccess PostgreSQL has already running: ${dbServerStatus}"
	return 0
    fi
    echo " [!] Proccess PostgreSQL is down: ${dbServerStatus}"
    return 1
}

function doCheckUserExist {
    expectExistUser=$1
    usr=$(grep "${expectExistUser}"  /etc/passwd)
    if [ -n "$usr" ]; then 
	echo " [!] ${expectExistUser} is exist"
	return 0
    fi
    echo " [!] ${expectExistUser} is not exist" 
    return 1
}

function doInstallUsers {
    echo " [=] Check or Install users"

    if ! doCheckUserExist "${REALM_ADMIN}"; then
	echo " [+] Add user ${REALM_ADMIN}"
	useradd -p "*" -m "${REALM_ADMIN}"
	chmod 700 "/home/${REALM_ADMIN}"
    fi

    if ! doCheckUserExist "${SSH_API_USER}"; then
	echo " [+] Add user ${SSH_API_USER}"
	useradd -p "*" -m "${SSH_API_USER}"
	chmod 700 "/home/${SSH_API_USER}"
    fi
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
