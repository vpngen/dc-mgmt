#!/bin/bash

BASE_DIR="."
OUTPUT_DIR="$BASE_DIR/.tmp"
CMD_DIR="$BASE_DIR/cmd"
SHARED_DIR="/data"
BUILD_OPTS="-buildvcs=false"

export GOOS=linux
export GOARCH=amd64
export CGO_ENABLE=0

# buildGo 
buildGo () {
  echo "[ $(dirname "$1") ]"
  go build -o $SHARED_DIR/$OUTPUT_DIR/"$(dirname "$1")" $BUILD_OPTS "$1" 
}  

# copyShellScripts {SRC} {DST} - copy files with .sh from SRC to DST directory
copyShellScripts() {
  find "$1" -type f -name "*.sh" -exec cp {} "$2" \;
}

if [ ! -d "$OUTPUT_DIR" ]; then
  mkdir -p "$OUTPUT_DIR"
fi

if [ -n "$(ls -A $OUTPUT_DIR)" ]; then
  rm -rf "$OUTPUT_DIR}/*"
fi

ssh-keyscan -t rsa github.com >> /etc/ssh/ssh_known_hosts
git config --global url."ssh://git@github.com/".insteadOf "https://github.com/"
go env -w GOPRIVATE=github.com/vpngen

# copyShellScripts "$CMD_DIR" "$OUTPUT_DIR"
#
#export -f buildGo
#find "$CMD_DIR"/ -type f -name "main.go" -exec bash -c "buildGo \"{}\"" \;
MAINGO_LIST=$(find "$CMD_DIR/" -type f -name "main.go")

for mainGo in $MAINGO_LIST; do
  buildGo "$mainGo"
done

find $OUTPUT_DIR -type f -exec mv {} $OUTPUT_DIR/cmd \;
