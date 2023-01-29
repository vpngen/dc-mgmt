#!/bin/sh

# interpret first argument as command
# pass rest args to scripts

printdef() {
    echo "Usage: <command> <args...>"
    exit 1
}

if [ $# -eq 0 ]; then 
    printdef
fi

cmd=${1}; shift
basedir=$(dirname $0)

if [ "xaddbrigade" = "x${cmd}" ]; then
    ${basedir}/addbrigade $@
elif [ "xdelbrigade" = "x${cmd}" ]; then
    ${basedir}/delbrigade $@
elif [ "xgetwasted" = "x${cmd}" ]; then
    ${basedir}/getwasted $@
else
    echo "Unknown command: ${cmd}"
    printdef
fi
