#!/bin/sh

# Read environment variables from the file
if [ -r "/etc/vg-dc-migr/subdomain-api.env" ]; then
        # shellcheck source=/dev/null
        . "/etc/vg-dc-migr/subdomain-api.env"
fi

# Check if the variables are set
if [ -z "$SUBDOMAIN_API_SERVER" ] || [ -z "$SUBDOMAIN_API_TOKEN" ]; then
  echo "Failed to load server or token from the environment file"
  exit 1
fi


printdef() {
        echo "Usage: <command> <options>" #-f <subdomain_file> [-n] -d <datacenter_id> -r <reservation_id>
        echo "  Commands:"
        echo "       create - Create domain migration for a reservation"
        уcho "       create -f <subdomain_file> [-n] -d <datacenter_id> -r <reservation_id>"
        echo "  Options:"
        echo "          -n : dry run"
        echo "          -dc : datacenter id"
        echo "          -r : reservation id"
        echo "          -f : subdomain file"
        echo "       release - Release domain migration for a reservation"
        echo "       release -f <subdomain_file> [-n] -r <reservation_id>"
        echo "          -n : dry run"
        echo "          -r : reservation id"
        echo "          -f : subdomain file"
        
        exit 1
}

release() {
        while [ $# -gt 0 ]; do
                case "$1" in
                        -f)
                                SUBDOMAIN_FILE="$2"
                                shift
                                ;;
                        -n)
                                DRY_RUN=yes
                                ;;
                        -r)
                                RESERVATION_ID="$2"
                                shift
                                ;;
                        *)
                                printdef "Unknown option: $1"
                                ;;
                esac
                shift
        done

        if [ -z "$SUBDOMAIN_FILE" ]; then
                echo "Missing subdomain_file"

                printdef
        fi

        if [ ! -s "$SUBDOMAIN_FILE" ]; then
                echo "Subdomain file not found: $SUBDOMAIN_FILE"

                exit 1
        fi

        # Create the request body for the migration
        MIGRATION_JSON=$(cat <<EOF
{
  "reservation_id": "${RESERVATION_ID}",
  "subdomains": $(jq -r '.subdomains' < "$SUBDOMAIN_FILE")
}
EOF
        )

        if [ -n "$DRY_RUN" ]; then
                echo "Dry run:"
                echo "${MIGRATION_JSON}"
                exit 0
        fi

        # Send the PUT request
        curl -X PATCH "http://${SUBDOMAIN_API_SERVER}/migration" \
             -H "Content-Type: application/json" \
             -H "Authorization: Bearer ${SUBDOMAIN_API_TOKEN}" \
             -d "${MIGRATION_JSON}"

}

create() {
        while [ $# -gt 0 ]; do
                case "$1" in
                        -f)
                                SUBDOMAIN_FILE="$2"
                                shift
                                ;;
                        -n)
                                DRY_RUN=yes
                                ;;
                        -dc)
                                DATACENTER_ID="$2"
                                shift
                                ;;
                        -r)
                                RESERVATION_ID="$2"
                                shift
                                ;;
                        *)
                                printdef "Unknown option: $1"
                                ;;
                esac
                shift
        done

        if [ -z "$SUBDOMAIN_FILE" ]; then
                echo "Missing subdomain_file"

                printdef
        fi

        if [ ! -s "$SUBDOMAIN_FILE" ]; then
                echo "Subdomain file not found: $SUBDOMAIN_FILE"

                exit 1
        fi

        # Create the request body for the migration
        MIGRATION_JSON=$(cat <<EOF
{
  "datacenter_id": "${DATACENTER_ID}",
  "reservation_id": "${RESERVATION_ID}",
  "subdomains": $(jq -r '.subdomains' < "$SUBDOMAIN_FILE")
}
EOF
        )

        if [ -n "$DRY_RUN" ]; then
                echo "Dry run:"
                echo "${MIGRATION_JSON}"
                exit 0
        fi

        # Send the PUT request
        curl -X PUT "http://${SUBDOMAIN_API_SERVER}/migration" \
             -H "Content-Type: application/json" \
             -H "Authorization: Bearer ${SUBDOMAIN_API_TOKEN}" \
             -d "${MIGRATION_JSON}"

}

while [ $# -gt 0 ]; do
        cmd="$1"
        shift
        case "$cmd" in
                create)
                        switch "$@"
                        ;;
                release)
                        delete "$@"
                        ;;
                -h|--help)
                        printdef
                        ;;
                *)
                        printdef "Unknown command: $1"
                        ;;
        esac
done