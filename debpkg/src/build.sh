#!/bin/sh

set -e

export CGO_ENABLED=0

go build -C dc-mgmt/cmd/addbrigade -o ../../../bin/addbrigade
go build -C dc-mgmt/cmd/addbrigade/gen -o ../../../../bin/gen
go build -C dc-mgmt/cmd/delbrigade -o ../../../bin/delbrigade
go build -C dc-mgmt/cmd/checkbrigade -o ../../../bin/checkbrigade
go build -C dc-mgmt/cmd/replacebrigadier -o ../../../bin/replacebrigadier
go build -C dc-mgmt/cmd/reset -o ../../../bin/reset
go build -C dc-mgmt/cmd/getwasted -o ../../../bin/getwasted
go build -C dc-mgmt/cmd/collectstats -o ../../../bin/collectstats
go build -C dc-mgmt/cmd/get_free_slots -o ../../../bin/get_free_slots
go build -C dc-mgmt/tools/cmd/dns-srv -o ../../../../bin/dns-srv
go build -C dc-mgmt/tools/cmd/dns-chk -o ../../../../bin/dns-chk
go build -C dc-mgmt/cmd/collectsnaps -o ../../../bin/collectsnaps

go install github.com/goreleaser/nfpm/v2/cmd/nfpm@latest

nfpm package --config "dc-mgmt/debpkg/nfpm.yaml" --target "${SHARED_BASE}/pkg" --packager deb

chown "${USER_UID}":"${USER_UID}" "${SHARED_BASE}/pkg/"*.deb
