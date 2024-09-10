#!/bin/sh

wd="$(dirname "$0")"

echo "Cleaning up generated files... ${wd}/gen-client/* ${wd}/gen-server/*"

rm -rf "${wd}/gen-client/"*
rm -rf "${wd}/gen-server/"*
