#!/bin/sh

set -e

CONFDIR=${CONFDIR:-"/etc/vgrealm"}
echo "confdir: ${CONFDIR}"
DBNAME=${DBNAME:-$(cat "${CONFDIR}"/dbname)}
echo "dbname: $DBNAME"
SCHEMA_PAIRS=${SCHEMA_PAIRS:-$(cat "${CONFDIR}"/pairs_schema)}
echo "pairs schema: $SCHEMA_PAIRS"
PAIRS_DBUSER=${PAIRS_DBUSER:-$(cat "${CONFDIR}"/pairs_dbuser)}
echo "pairs user: $PAIRS_DBUSER"
SCHEMA_BRIGADES=${SCHEMA_BRIGADES:-$(cat "${CONFDIR}"/brigades_schema)}
echo "brigades schema: $SCHEMA_BRIGADES"
BRIGADES_DBUSER=${BRIGADES_DBUSER:-$(cat "${CONFDIR}"/brigades_dbuser)}
echo "brigades user: $BRIGADES_DBUSER"

set -x

sudo -i -u postgres psql -v -d "${DBNAME}" \
    --set schema_pairs_name="${SCHEMA_PAIRS}" \
    --set schema_brigades_name="${SCHEMA_BRIGADES}" \
    --set pairs_dbuser="${PAIRS_DBUSER}" \
    --set brigades_dbuser="${BRIGADES_DBUSER}" \
    < $(dirname "$0")/000-install.sql
