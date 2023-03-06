#!/bin/bash

PREINSTALL_SCRIPT_NAME="$0"
PROJECT_NAME="realm-admin"




function doRemoveTempDir {
    echo "doRemoveTempDir"

    ls -l "/tmp/${INSTALLATION_TEMP_DIR}"
}

function doRemoveDebFile {
    echo "doRemoveDebFile"
    unset -v INSTALLATION_TEMP_DIR

    find ./ -type f -iname "${PROJECT_NAME}.*\.deb" -exec rm {} \;
}

doRemoveTempDir
doRemoveDebFile
