#!/bin/bash

GOLANG_VER="$(grep -e "^go\s" go.mod | awk '{print $2}')"
WORK_DIR="/data"

docker run --rm \
  -v "${PWD}":"${WORK_DIR}" \
  -w "${WORK_DIR}" \
  golang:"${GOLANG_VER}" make compile

