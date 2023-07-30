#!/bin/sh

set -e

BASEDIR=${BASEDIR:-$(pwd)}; export BASEDIR
echo "[i] basedir: ${BASEDIR}"
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

"${BASEDIR}"/sql/__clean.sh
"${BASEDIR}"/sql/install.sh

${BASEDIR}/cmd/addnet/add_endpoint_net.sh 185.35.220.0/24 185.35.220.1
${BASEDIR}/cmd/addnet/add_endpoint_net.sh 185.35.223.0/24 185.35.223.1
${BASEDIR}/cmd/addnet/add_endpoint_net.sh 31.177.88.0/24 31.177.88.1

${BASEDIR}/cmd/addnet/add_private_net.sh 192.168.100.0/24

${BASEDIR}/cmd/addnet/add_cgnat_net.sh 100.64.0.0/22
${BASEDIR}/cmd/addnet/add_cgnat_net.sh 100.74.0.0/22
${BASEDIR}/cmd/addnet/add_cgnat_net.sh 100.84.0.0/22

${BASEDIR}/cmd/addnet/add_ula_net.sh fd01:f1e2::/32
${BASEDIR}/cmd/addnet/add_ula_net.sh fd10:7c54::/32
${BASEDIR}/cmd/addnet/add_ula_net.sh fd42:8bfe::/32

${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:10::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:20::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:30::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:40::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:50::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:60::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:70::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:80::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd02:90::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:1::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:2::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:3::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:4::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:5::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:6::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:7::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:8::/112
${BASEDIR}/cmd/addnet/add_keydesk_net.sh fd76:9::/112

${BASEDIR}/cmd/addpair/add_pair.sh "$(uuidgen)" 192.168.100.101 185.35.220.2 185.35.220.3 185.35.220.4 185.35.220.5 185.35.220.6 185.35.223.19 185.35.223.2 185.35.223.3 31.177.88.2
${BASEDIR}/cmd/addpair/add_pair.sh "$(uuidgen)" 192.168.100.102 185.35.220.7 185.35.220.8 185.35.220.9 185.35.220.10 185.35.220.11 185.35.223.4 185.35.223.5 185.35.223.6 31.177.88.3
${BASEDIR}/cmd/addpair/add_pair.sh "$(uuidgen)" 192.168.100.103 185.35.220.12 185.35.220.13 185.35.220.14 185.35.220.15 185.35.220.16 185.35.223.7 185.35.223.8 185.35.223.9 31.177.88.4
${BASEDIR}/cmd/addpair/add_pair.sh "$(uuidgen)" 192.168.100.104 185.35.220.17 185.35.220.18 185.35.220.19 185.35.220.31 185.35.220.20 185.35.223.10 185.35.223.11 185.35.223.12 31.177.88.5
${BASEDIR}/cmd/addpair/add_pair.sh "$(uuidgen)" 192.168.100.105 185.35.220.21 185.35.220.22 185.35.220.23 185.35.220.24 185.35.220.25 185.35.223.13 185.35.223.14 185.35.223.15 31.177.88.6
${BASEDIR}/cmd/addpair/add_pair.sh "$(uuidgen)" 192.168.100.106 185.35.220.26 185.35.220.27 185.35.220.28 185.35.220.29 185.35.220.30 185.35.223.16 185.35.223.17 185.35.223.18 31.177.88.7

for addr in 185.35.220.2 185.35.220.3 185.35.220.4 185.35.220.5 185.35.220.6 185.35.223.19 185.35.223.2 185.35.223.3 31.177.88.2; do
        domain=$(echo "$addr" | awk -F. '{OFS="."; print $4,$3,$2,$1,"in-addr.arpa"}')
        ${BASEDIR}/cmd/addpair/add_domain.sh "$addr" "$domain"
done 

for addr in 185.35.220.7 185.35.220.8 185.35.220.9 185.35.220.10 185.35.220.11 185.35.223.4 185.35.223.5 185.35.223.6 31.177.88.3; do
        domain=$(echo "$addr" | awk -F. '{OFS="."; print $4,$3,$2,$1,"in-addr.arpa"}')
        ${BASEDIR}/cmd/addpair/add_domain.sh "$addr" "$domain"
done 
for addr in 185.35.220.12 185.35.220.13 185.35.220.14 185.35.220.15 185.35.220.16 185.35.223.7 185.35.223.8 185.35.223.9 31.177.88.4; do
        domain=$(echo "$addr" | awk -F. '{OFS="."; print $4,$3,$2,$1,"in-addr.arpa"}')
        ${BASEDIR}/cmd/addpair/add_domain.sh "$addr" "$domain"
done 
for addr in 185.35.220.17 185.35.220.18 185.35.220.19 185.35.220.31 185.35.220.20 185.35.223.10 185.35.223.11 185.35.223.12 31.177.88.5; do
        domain=$(echo "$addr" | awk -F. '{OFS="."; print $4,$3,$2,$1,"in-addr.arpa"}')
        ${BASEDIR}/cmd/addpair/add_domain.sh "$addr" "$domain"
done 
for addr in 185.35.220.21 185.35.220.22 185.35.220.23 185.35.220.24 185.35.220.25 185.35.223.13 185.35.223.14 185.35.223.15 31.177.88.6; do
        domain=$(echo "$addr" | awk -F. '{OFS="."; print $4,$3,$2,$1,"in-addr.arpa"}')
        ${BASEDIR}/cmd/addpair/add_domain.sh "$addr" "$domain"
done 
for addr in 185.35.220.26 185.35.220.27 185.35.220.28 185.35.220.29 185.35.220.30 185.35.223.16 185.35.223.17 185.35.223.18 31.177.88.7; do
        domain=$(echo "$addr" | awk -F. '{OFS="."; print $4,$3,$2,$1,"in-addr.arpa"}')
        ${BASEDIR}/cmd/addpair/add_domain.sh "$addr" "$domain"
done 

ON_ERROR_STOP=yes psql -d "${DBNAME}" --set schema_name="${SCHEMA_PAIRS}" <<EOF
    UPDATE :"schema_name".pairs SET is_active=true;
EOF