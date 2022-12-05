#!/bin/sh

# interpret first argument as command
# pass rest args to scripts

if [ $# -eq 0 ]; then 
    echo "Usage: $0 <command> <args...>"
    exit 1
fi

cmd=${1}; shift
basedir=$(dirname $0)

if [ "xadd" = "x${cmd}" ]; then
    ${basedir}/addbrigade $@
elif [ "xdel" = "x${cmd}" ]; then
    ${basedir}/delbrigade $@
else
    echo "Unknown command: ${cmd}"
    echo "Usage: $0 <command> <args...>"
    exit 1
fi
