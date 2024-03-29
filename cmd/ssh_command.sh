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
basedir=$(dirname "$0")

if [ -s  "/etc/vg-dc-mgmt/dc-name.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-mgmt/dc-name.env"
fi

if [ -s "/etc/vg-dc-vpnapi/modbrigade.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-vpnapi/modbrigade.env"
fi

if [ -s "/etc/vg-dc-vpnapi/creation.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-vpnapi/creation.env"
fi

vpn_works_keysesks_sync() {
        /usr/bin/flock -x -E 0 -n /tmp/modbrigade.lock "${basedir}"/vpn-works-keydesks-sync.sh 2>&1 | /usr/bin/logger -p local0.notice -t KDSYNC
}

delegation_sync() {
        /usr/bin/flock -x -E 0 -n /tmp/modbrigade.lock "${basedir}"/delegation-sync.sh 2>&1 | /usr/bin/logger -p local0.notice -t DOMSYNC
}

if [ "addbrigade" = "${cmd}" ]; then
        DC_ID="${DC_ID}" \
        DC_NAME="${DC_NAME}" \
        SUBDOMAIN_API_SERVER="${SUBDOMAIN_API_SERVER}" \
        SUBDOMAIN_API_TOKEN="${SUBDOMAIN_API_TOKEN}" \
        DELEGATION_SYNC_CONNECT="${DELEGATION_SYNC_CONNECT}" \
        KEYDESK_ADDRESS_SYNC_CONNECT="${KEYDESK_ADDRESS_SYNC_CONNECT}" \
        KEYDESK_DOMAIN="${KEYDESK_DOMAIN}" \
        KEYDESK_NAMESERVERS="${KEYDESK_NAMESERVERS}" \
        DOMAIN_NAMESERVERS="${DOMAIN_NAMESERVERS}" \
        WIREGUARD_CONFIGS="${WIREGUARD_CONFIGS}" \
        OVC_CONFIGS="${OVC_CONFIGS}" \
        OUTLINE_CONFIGS="${OUTLINE_CONFIGS}" \
        IPSEC_CONFIGS="${IPSEC_CONFIGS}" \
        flock -x -E 1 -w 60 /tmp/modbrigade.lock "${basedir}"/addbrigade "$@"
        #vpn_works_keysesks_sync
        #delegation_sync
elif [ "delbrigade" = "${cmd}" ]; then
        DC_ID="${DC_ID}" \
        DC_NAME="${DC_NAME}" \
        SUBDOMAIN_API_SERVER="${SUBDOMAIN_API_SERVER}" \
        SUBDOMAIN_API_TOKEN="${SUBDOMAIN_API_TOKEN}" \
        DELEGATION_SYNC_CONNECT="${DELEGATION_SYNC_CONNECT}" \
        KEYDESK_ADDRESS_SYNC_CONNECT="${KEYDESK_ADDRESS_SYNC_CONNECT}" \
        KEYDESK_DOMAIN="${KEYDESK_DOMAIN}" \
        KEYDESK_NAMESERVERS="${KEYDESK_NAMESERVERS}" \
        DOMAIN_NAMESERVERS="${DOMAIN_NAMESERVERS}" \
        WIREGUARD_CONFIGS="${WIREGUARD_CONFIGS}" \
        OVC_CONFIGS="${OVC_CONFIGS}" \
        OUTLINE_CONFIGS="${OUTLINE_CONFIGS}" \
        IPSEC_CONFIGS="${IPSEC_CONFIGS}" \
        flock -x -E 1 -w 60 /tmp/modbrigade.lock "${basedir}"/delbrigade "$@"
        #vpn_works_keysesks_sync
        #delegation_sync
elif [ "replacebrigadier" = "${cmd}" ]; then
        DC_ID="${DC_ID}" \
        DC_NAME="${DC_NAME}" \
        REPLACE_WIREGUARD_CONFIGS="${REPLACE_WIREGUARD_CONFIGS}" \
        REPLACE_OVC_CONFIGS="${REPLACE_OVC_CONFIGS}" \
        REPLACE_OUTLINE_CONFIGS="${REPLACE_OUTLINE_CONFIGS}" \
        REPLACE_IPSEC_CONFIGS="${REPLACE_IPSEC_CONFIGS}" \
        "${basedir}"/replacebrigadier "$@"
elif [ "getwasted" = "${cmd}" ]; then
        "${basedir}"/getwasted "$@"
elif [ "checkbrigade" = "${cmd}" ]; then
        "${basedir}"/checkbrigade "$@"
elif [ "get_free_slots" = "${cmd}" ]; then
    "${basedir}"/get_free_slots "$@"
else
    echo "Unknown command: ${cmd}"
    printdef
fi
