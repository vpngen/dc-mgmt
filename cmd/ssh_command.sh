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
        PROTO0_CONFIGS="${PROTO0_CONFIGS}" \
        VIP_INTERNAL_NETWORKS="${VIP_INTERNAL_NETWORKS}" \
        "${basedir}"/addbrigade "$@"
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
        PROTO0_CONFIGS="${PROTO0_CONFIGS}" \
        "${basedir}"/delbrigade "$@"
elif [ "replacebrigadier" = "${cmd}" ]; then
        DC_ID="${DC_ID}" \
        DC_NAME="${DC_NAME}" \
        REPLACE_WIREGUARD_CONFIGS="${REPLACE_WIREGUARD_CONFIGS}" \
        REPLACE_OVC_CONFIGS="${REPLACE_OVC_CONFIGS}" \
        REPLACE_OUTLINE_CONFIGS="${REPLACE_OUTLINE_CONFIGS}" \
        REPLACE_IPSEC_CONFIGS="${REPLACE_IPSEC_CONFIGS}" \
        REPLACE_PROTO0_CONFIGS="${REPLACE_PROTO0_CONFIGS}" \
        "${basedir}"/replacebrigadier "$@"
elif [ "getwasted" = "${cmd}" ]; then
        "${basedir}"/getwasted "$@"
elif [ "checkbrigade" = "${cmd}" ]; then
        "${basedir}"/checkbrigade "$@"
elif [ "get_free_slots" = "${cmd}" ]; then
        "${basedir}"/get_free_slots "$@"
elif [ "${cmd}" = "vipon" ]; then
        "${basedir}/turnon-vip" "$@" "-on"
elif [ "${cmd}" = "vipoff" ]; then
        "${basedir}/turnon-vip" "$@" "-off"
else
    echo "Unknown command: ${cmd}"
    printdef
fi
