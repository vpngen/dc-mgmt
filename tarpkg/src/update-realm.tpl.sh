#!/bin/sh

set -e

CONFDIR=${CONFDIR:-"/etc/vgrealm"}

DBNAME=${DBNAME:-"vgrealm"}
PAIRS_DBUSER=${PAIRS_DBUSER:-"vgrealm"}
BRIGADES_DBUSER=${BRIGADES_DBUSER:-"vgrealm"}
PAIRS_SCHEMA=${PSCHEMA:-"pairs"}
BRIGADES_SCHEMA=${BSCHEMA:-"brigades"}

REALM_ADMIN=${REALM_ADMIN:-"vgrealm"}
SSH_API_USER=${SSH_API_USER:="_valera_"}

FORCE_INSTALL=$1

if [ "x" != "x${FORCE_INSTALL}" ]; then
        export DEBIAN_FRONTEND=noninteractive
        apt-get update; apt-get dist-upgrade -y; apt-get autoremove -y
        apt-get install -q -y --no-install-recommends postgresql postgresql-contrib

        systemctl start --now postgresql
fi

# Extract files

INSTALL_DIR="/opt/__install__"
install -g root -o root -m 0500 -d "${INSTALL_DIR}"
awk '/^__PAYLOAD_BEGINS__/ { print NR + 1; exit 0; }' $0 | xargs -I {} tail -n +{} $0 | base64 -d | tar -xzp -C ${INSTALL_DIR} >> /install.log 2>&1

# Install realm-admin

if [ "x" != "x${FORCE_INSTALL}" ]; then
        useradd -p "*" -m "${REALM_ADMIN}"
        chmod 700 "/home/${REALM_ADMIN}"

        install -o root -g "${REALM_ADMIN}" -m 0010 -d "${CONFDIR}"
        install -o root -g "${REALM_ADMIN}" -m 0010 -d "${CONFDIR}"
        install -o root -g "${REALM_ADMIN}" -m 0010 -d "${CONFDIR}/utils"
        install -o root -g "${REALM_ADMIN}" -m 0010 -d "${CONFDIR}/cmd"

        echo "${DBNAME}" > "/tmp/dbname"
        echo "${PAIRS_DBUSER}" > "/tmp/pairs_dbuser"
        echo "${BRIGADES_DBUSER}" > "/tmp/brigades_dbuser"
        echo "${PAIRS_SCHEMA}" > "/tmp/pairs_schema"
        echo "${BRIGADES_SCHEMA}" > "/tmp/brigades_schema"

        install -o root -g "${REALM_ADMIN}" -m 040 "/tmp/dbname" "${CONFDIR}/dbname"
        install -o root -g "${REALM_ADMIN}" -m 040 "/tmp/pairs_dbuser" "${CONFDIR}/pairs_dbuser"
        install -o root -g "${REALM_ADMIN}" -m 040 "/tmp/brigades_dbuser" "${CONFDIR}/brigades_dbuser"
        install -o root -g "${REALM_ADMIN}" -m 040 "/tmp/pairs_schema" "${CONFDIR}/pairs_schema"
        install -o root -g "${REALM_ADMIN}" -m 040 "/tmp/brigades_schema" "${CONFDIR}/brigades_schema"
fi

# Init database

if [ "x" != "x${FORCE_INSTALL}" ]; then
        sudo -i -u postgres psql <<EOF
CREATE DATABASE ${DBNAME};
EOF
        sudo -i -u postgres "DBNAME=${DBNAME} ${INSTALL_DIR}/install.sh"
fi

install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/add_endpoint_net.sh" /opt/vgrealm/utils/
install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/add_private_net.sh" /opt/vgrealm/utils/
install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/add_cgnat_net.sh" /opt/vgrealm/utils/
install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/add_ula_net.sh" /opt/vgrealm/utils/
install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/add_keydesk_net.sh" /opt/vgrealm/utils/
install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/gen" /opt/vgrealm/utils/
install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/ssh_command.sh" /opt/vgrealm/cmd/
install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/addbrigade" /opt/vgrealm/cmd/
install -o root -g "${REALM_ADMIN}" -m 050 "${INSTALL_DIR}/bin/delbrigade" /opt/vgrealm/cmd/


if [ "x" != "x${FORCE_INSTALL}" ]; then
        useradd -p "*" -m "${SSH_API_USER}"
        chmod 700 "/home/${SSH_API_USER}"

        install -g root -o root -m 644 "${INSTALL_DIR}/${SSH_API_USER}" "/etc/sudoers.d/${SSH_API_USER}"
        install -g ${SSH_API_USER} -o ${SSH_API_USER} -m 0700 -d "/home/${SSH_API_USER}/.ssh"
        install -g ${SSH_API_USER} -o ${SSH_API_USER} -m 600 "${INSTALL_DIR}/authorized_keys" "/home/${SSH_API_USER}/.ssh/authorized_keys"
fi

# Cleanup

rm -rf "${INSTALL_DIR}"

exit 0
__PAYLOAD_BEGINS__
