#!/bin/sh

set -e

DBNAME=${DBNAME:-"vgrealm"}
echo "dbname: $DBNAME"
SCHEMA_PAIRS=${SCHEMA_PAIRS:-"pairs"}
echo "pairs schema: $SCHEMA_PAIRS"
PAIRS_DBUSER=${PAIRS_DBUSER:-"vgradm"}
echo "pairs user: $PAIRS_DBUSER"
SCHEMA_BRIGADES=${SCHEMA_BRIGADES:-"brigades"}
echo "brigades schema: $SCHEMA_BRIGADES"
BRIGADES_DBUSER=${BRIGADES_DBUSER:-"vgrealm"}
echo "brigades user: $BRIGADES_DBUSER"
SCHEMA_STATS=${SCHEMA_STATS:-"stats"}
echo "stats schema: $SCHEMA_STATS"
STATS_DBUSER=${STATS_DBUSER:-"vgstats"}
echo "brigades user: $STATS_DBUSER"
SNAPS_DBUSER=${SNAPS_DBUSER:-"vgsnaps"}
echo "brigades user: $SNAPS_DBUSER"
MIGR_DBUSER=${MIGR_DBUSER:-"vgmigr"}
echo "brigades user: $MIGR_DBUSER"

set -x

cat "$(dirname "$0")"/init/*.sql | sudo -i -u postgres psql -v -d "${DBNAME}" \
    --set schema_pairs_name="${SCHEMA_PAIRS}" \
    --set pairs_dbuser="${PAIRS_DBUSER}" \
    --set schema_brigades_name="${SCHEMA_BRIGADES}" \
    --set brigades_dbuser="${BRIGADES_DBUSER}" \
    --set schema_stats_name="${SCHEMA_STATS}" \
    --set stats_dbuser="${STATS_DBUSER}" \
    --set snaps_dbuser="${SNAPS_DBUSER}" \
    --set migr_dbuser="${MIGR_DBUSER}" 

cat "$(dirname "$0")"/patches/*.sql | sudo -i -u postgres psql -v -d "${DBNAME}" \
    --set schema_pairs_name="${SCHEMA_PAIRS}" \
    --set pairs_dbuser="${PAIRS_DBUSER}" \
    --set schema_brigades_name="${SCHEMA_BRIGADES}" \
    --set brigades_dbuser="${BRIGADES_DBUSER}" \
    --set schema_stats_name="${SCHEMA_STATS}" \
    --set stats_dbuser="${STATS_DBUSER}" \
    --set snaps_dbuser="${SNAPS_DBUSER}" \
    --set migr_dbuser="${MIGR_DBUSER}" 
