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
    /usr/bin/flock -x -E 0 -n /tmp/kdsync.lock ${basedir}/kdsync.sh 2>&1 | /usr/bin/logger -p local0.notice -t KDSYNC
elif [ "xdelbrigade" = "x${cmd}" ]; then
    ${basedir}/delbrigade $@
    /usr/bin/flock -x -E 0 -n /tmp/kdsync.lock ${basedir}/kdsync.sh 2>&1 | /usr/bin/logger -p local0.notice -t KDSYNC
elif [ "xgetwasted" = "x${cmd}" ]; then
    ${basedir}/getwasted $@
else
    echo "Unknown command: ${cmd}"
    printdef
fi
