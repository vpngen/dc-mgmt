#!/bin/sh

set -e

export CGO_ENABLED=0

go install github.com/vpngen/realm-admin/cmd/delbrigade@latest
go install github.com/vpngen/realm-admin/cmd/addbrigade@latest
go install github.com/vpngen/realm-admin/cmd/addbrigade/gen@latest

# git clone ssh://git@github.com/vpngen/realm-admin 
cp realm-admin/cmd/addnet/*.sh bin/
cp realm-admin/cmd/addpair/*.sh bin/
cp realm-admin/cmd/*.sh bin/
cp -r realm-admin/install .

cp -f realm-deploy/src/_valera_ .

rm -f /data/extract-realm.sh
cp -f realm-admin/tarpkg/src/update-realm.tpl.sh /data/update-realm.sh

echo "command=\"sudo -u vgrealm -g vgrealm /opt/vgrealm/cmd/ssh_command.sh \${SSH_ORIGINAL_COMMAND}\",no-port-forwarding,no-X11-forwarding,no-agent-forwarding,no-pty ${AUTH_KEY}" > authorized_keys

tar -zcf - \
        bin/delbrigade \
        bin/addbrigade \
        bin/gen \
        bin/add_endpoint_net.sh \
        bin/add_cgnat_net.sh \
        bin/add_ula_net.sh \
        bin/add_private_net.sh \
        bin/add_keydesk_net.sh \
        bin/add_pair.sh \
        bin/ssh_command.sh \
        install \
        _valera_ \
        | base64 >> /data/update-realm.sh

chown ${USER_UID}:${USER_UID} /data/update-realm.sh

