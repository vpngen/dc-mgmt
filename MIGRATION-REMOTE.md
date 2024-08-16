# Migration between datacenters via encrypted snapshots

This document describes the process of migrating a set of brigades between two data centers using encrypted snapshots.

## Required special configuration in the data center

* Well configured datacenters
* Source datacenter private RSA key online

## Required special configuration in the ministry

* Well configured ministry
* Authority private RSA key online
* Master shuffler key online (target datacenter shuffler key if applicable)

## STEP 1: Create a snapshot

* Use `vgsnaps` source datacenter account to create a snapshot
* Calculate maintenance mode end time if necessary
  * Example: `date -d "2038-01-01" +%s`
  * Result: `2145916800`
* Create a dedicated snapshot with good mnemonic
  * Example: `/opt/vg-dc-snaps/collectsnaps.sh -tag 4.33.222.0-migr -net 4.33.222.0/24 -ad -mnt 2145916800`
  * Result file: `/vg-snapshots/4.33.222.0-migr/4.33.222.0-migr-20000101-000000.json`
* Check the snapshot for errors
  * Example: `jq -r '"errors_count=\(.errors_count), total_count=\(.total_count)"' < /vg-snapshots/4.33.222.0-migr/4.33.222.0-migr-20000101-000000.json`
  * Result: `errors_count=0, total_count=111`

## STEP 2: Create a reservation

* Use `vgmigr` target datacenter account to create a reservation
* Define if you need a reservation in the specific service network or/and external network (optional)
  * `10.10.10.0/24` service network and `5.33.222.0/24` external network in this example
* Create reservation with specified number of slots
  * Example: `/opt/vg-dc-snaps/create_reservation.sh create -in 10.10.10.0/24 -en 5.33.222.0/24 111`
  * Result: `reservation: ab12cd34-5678-90ab-cdef-1234567890ab`
* Generate a reservation config with good mnemonic
  * Example: `/opt/vg-dc-snaps/create_reservation.sh genconf ab12cd34-5678-90ab-cdef-1234567890ab > 4.33.222.0-migr-reserv-ab12cd34.json` (use the first 8 characters of the reservation ID as suffix)

## STEP 3: Prepare the snapshot for migration

* Use `vgmigr` source datacenter account to prepare the snapshot for migration
* Prepare the authority RSA public key in the SSH-fingerprint format (now the authority key is a special RSA key in the ministry)
  * Example: `SHA256:B3JD/EmrE1uKHirw2vn2ndAV1iWn+/uAGyYD0MtPJO0`
* Prepare the datacenter RSA private key online 
* Create a prepared snapshot with good mnemonic
  * Example: `/opt/vg-dc-snaps/snap_prepare -fp "SHA256:B3JD/EmrE1uKHirw2vn2ndAV1iWn+/uAGyYD0MtPJO0" < /vg-snapshots/4.33.222.0-migr/4.33.222.0-migr-20000101-000000.json > 4.33.222.0-migr-prepared-20000101-000000.json`

## STEP 4: Copy the prepared snapshot and reservation config to the ministry

* Use `vg_head_migr` ministry account to store the prepared snapshot from source datacenter and reservation config form target datacenter

## STEP 5: Create a migration plan

* Use `vg_head_migr` ministry account to create a migration plan
* Prepare the datacenter RSA public key in the SSH-fingerprint format (now the datacenter key is a special RSA key in the ministry)
  * Example: `SHA256:YfhyykjnOBiYI6rZ/iD/TgPAc+/mlqnkadmUyXgYUkU`
  * Ussualy it is in the `/etc/vg-dc-mgmt/realmfp.env` file in the datacenter
* Define if you need a mirror migration (optional) with flag `-mirror`. Only same IP addresses on reservation and snapshot will be migrated.
* Create a migration plan with good mnemonic
  * Example: `/opt/vg-head-vpnapi/recodesnaps -tfp SHA256:YfhyykjnOBiYI6rZ/iD/TgPAc+/mlqnkadmUyXgYUkU -c 4.33.222.0-migr-reserv-ab12cd34.json -in 4.33.222.0-migr-prepared-20000101-000000.json -out 4.33.222.0-migr-plan-20000101-000000.json`

## STEP 6: Create a spare brigades in the ministry

* Use `/opt/vg-head-vpnapi/vg_head_migr` ministry account to create a spare datacenter records
* Prepare the target datacenter ID
  * Ussualy it is in the `/etc/vg-dc-mgmt/dc_id.env` file in the datacenter
* Create a spare datacenter records 
  * Example: `/opt/vg-head-vpnapi/switch_remote_migr.sh create -f 4.33.222.0-migr-prepared-20000101-000000.json -t bc12de34-5678-90ab-cdef-1234567890ab`

## STEP 7: Copy the migration plan back to the datacenter

* Use `vgmigr` target datacenter account to store the migration plan

## STEP 8: Restore brigades with the migration plan

* Use `vgmigr` target datacenter account to restore brigades with the migration plan
* Prepare the reservation ID wich was used in the migration plan
  * Example: `ab12cd34-5678-90ab-cdef-1234567890ab`
* Restore brigades
  * Example: `/opt/vg-dc-snaps/restoresnaps -r ab12cd34-5678-90ab-cdef-1234567890ab -f 4.33.222.0-migr-plan-20000101-000000.json > tee log-restore-4.33.222.0-migr-plan-20000101-000000-$(date +"%Y%m%d%H%M").log`
* Check the restore log for errors

## STEP 9: Accept the new brigades instances in the target datacenter

* Use `vgmigr` target datacenter account to switch the brigades instances to the new ones
* Accept the new brigades instances
  * Example: `/opt/vg-dc-snaps/accept_remote_migr.sh -r ab12cd34-5678-90ab-cdef-1234567890ab -f 4.33.222.0-migr-plan-20000101-000000.json`
* Use `vgvpnapi` target datacenter account to sync subdomains
* Sync subdomains with subdomain management API
  * Example: `SSH_KEY=~/.ssh/id_ed25519 /opt/vg-dc-vpnapi/delegation-sync.sh`
* At this point, there are two records for each subdomain in the set. One for the old brigade and one for the new brigade

## STEP 10: Wait for the DNS propagation

* Wait for the DNS propagation 24 hours

## STEP 11: Switch a spare brigades in the ministry

* Use `/opt/vg-head-vpnapi/vg_head_migr` ministry account to create a spare datacenter records
* Prepare the target datacenter ID
  * Ussualy it is in the `/etc/vg-dc-mgmt/dc_id.env` file in the datacenter
* Create a spare datacenter records 
  * Example: `/opt/vg-head-vpnapi/switch_remote_migr.sh switch -f 4.33.222.0-migr-prepared-20000101-000000.json -t bc12de34-5678-90ab-cdef-1234567890ab`

## STEP 12: Release subdomains in the source datacenter and send then to the reservation in the target datacenter

* Use `vgmigr` source datacenter account to release subdomains in the source datacenter
* Release subdomains
  * Example: `/opt/vg-dc-snaps/release_remote_migr.sh -f 4.33.222.0-migr-prepared-20000101-000000.json -o 4.33.222.0-migr-subdomains.json`
* Use `vgmigr` source datacenter account to move subdomains ownership from the source datacenter to the target reservation
* Prepare the target datacenter ID
  * Ussualy it is in the `/etc/vg-dc-mgmt/dc_id.env` file in the datacenter
* Move subdomains ownership
  * Example: `/opt/vg-dc-snaps/subdomain_reservation.sh create -dc bc12de34-5678-90ab-cdef-1234567890ab -r ab12cd34-5678-90ab-cdef-1234567890ab -f 4.33.222.0-migr-subdomains.json`
* Use `vgvpnapi` source datacenter account to sync subdomains
* Sync subdomains with subdomain management API
  * Example: `SSH_KEY=~/.ssh/id_ed25519 /opt/vg-dc-vpnapi/delegation-sync.sh`

## STEP 13: Copy the subdomains file from the source datacenter to the target datacenter

* Use `vgmigr` source datacenter account to collect the subdomains file
* Use `vgmigr` target datacenter account to store the subdomains file in the target datacenter
* Copy the subdomains file from the source datacenter to the target datacenter

# STEP 14: Accept the new subdomains in the target datacenter

* Use `vgmigr` target datacenter account to accept the subdomains in the target datacenter
* Accept the new subdomains
  * Example: `/opt/vg-dc-snaps/subdomain_reservation.sh release -r ab12cd34-5678-90ab-cdef-1234567890ab -f 4.33.222.0-migr-subdomains.json`

## STEP 15: Clean up in the source datacenter

* Use `vgmigr` source datacenter account to clean up
* Clean up the old brigades instances
  * Example: `/opt/vg-dc-snaps/switch_local_migr.sh delete -r "" -f 4.33.222.0-migr-prepared-20000101-000000.json` (not implemented yet)
  * NOTE: Possibly need to develop a new script for this with ability to turn off control nodes

## STEP 16: Clean up in the ministry

* Use `vg_head_migr` ministry account to clean up
* Prepare the source datacenter ID
  * Ussualy it is in the `/etc/vg-dc-mgmt/dc_id.env` file in the datacenter
* Clean up the spare datacenter records
  * Example: `/opt/vg-head-vpnapi/switch_remote_migr.sh delete -s 765d4321-5678-90ab-cdef-1234567890ab -f 4.33.222.0-migr-prepared-20000101-000000.json`

## STEP 17: Clean up in the target datacenter

* Clean up the reservation
  * Example: `/opt/vg-dc-snaps/create_reservation.sh delete -f ab12cd34-5678-90ab-cdef-1234567890ab`
  