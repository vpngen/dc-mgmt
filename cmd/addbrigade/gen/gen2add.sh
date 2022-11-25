#!/bin/sh

set -e

args=""
dir=$(dirname $0)

crerate_args() {
    args="-id ${1} -name ${2} -person ${3} -desc ${4} -url ${5}"
}

crerate_args $(${dir}/gen)

echo "${args}"