# Local migration via encrypted snapshots

This document describes the process of migrating a set of brigades within a single data center using encrypted snapshots.

## Required special configuration in the data center

* Well configured datacenter
* Datacenter private RSA key online

## Required special configuration in the ministry

* Well configured ministry
* Authority private RSA key online
* Master shuffler key online

## STEP 1: Create a snapshot

* Use `vgsnaps` datacenter account to create a snapshot
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

* Use `vgmigr` datacenter account to create a reservation
* Define if you need a reservation in the specific service network or/and external network (optional)
  * `10.10.10.0/24` service network and `5.33.222.0/24` external network in this example
* Create reservation with specified number of slots
  * Example: `/opt/vg-dc-snaps/create_reservation.sh create -in 10.10.10.0/24 -en 5.33.222.0/24 111`
  * Result: `reservation: ab12cd34-5678-90ab-cdef-1234567890ab`
* Generate a reservation config with good mnemonic
  * Example: `/opt/vg-dc-snaps/create_reservation.sh genconf ab12cd34-5678-90ab-cdef-1234567890ab > 4.33.222.0-migr-reserv-ab12cd34.json` (use the first 8 characters of the reservation ID as suffix)

## STEP 3: Prepare the snapshot for migration

* Use `vgmigr` datacenter account to prepare the snapshot for migration
* Prepare the authority RSA public key in the SSH-fingerprint format (now the authority key is a special RSA key in the ministry)
  * Example: `SHA256:B3JD/EmrE1uKHirw2vn2ndAV1iWn+/uAGyYD0MtPJO0`
* Prepare the datacenter RSA private key online 
* Create a prepared snapshot with good mnemonic
  * Example: `/opt/vg-dc-snaps/snap_prepare -fp "SHA256:B3JD/EmrE1uKHirw2vn2ndAV1iWn+/uAGyYD0MtPJO0" < /vg-snapshots/4.33.222.0-migr/4.33.222.0-migr-20000101-000000.json > 4.33.222.0-migr-prepared-20000101-000000.json`

## STEP 4: Copy the prepared snapshot and reservation config to the ministry

* Use `vg_head_migr` ministry account to store the prepared snapshot and reservation config

## STEP 5: Create a migration plan

* Use `vg_head_migr` ministry account to create a migration plan
* Prepare the datacenter RSA public key in the SSH-fingerprint format (now the datacenter key is a special RSA key in the ministry)
  * Example: `SHA256:YfhyykjnOBiYI6rZ/iD/TgPAc+/mlqnkadmUyXgYUkU`
  * Ussualy it is in the `/etc/vg-dc-mgmt/realmfp.env` file in the datacenter
* Create a migration plan with good mnemonic
  * Example: `/opt/vg-head-vpnapi/recodesnaps -tfp SHA256:YfhyykjnOBiYI6rZ/iD/TgPAc+/mlqnkadmUyXgYUkU -c 4.33.222.0-migr-reserv-ab12cd34.json -in 4.33.222.0-migr-prepared-20000101-000000.json  -out 4.33.222.0-migr-plan-20000101-000000.json`

## STEP 6: Copy the migration plan back to the datacenter

* Use `vgmigr` datacenter account to store the migration plan

## STEP 7: Restore brigades with the migration plan

* Use `vgmigr` datacenter account to restore brigades with the migration plan
* Prepare the reservation ID wich was used in the migration plan
  * Example: `ab12cd34-5678-90ab-cdef-1234567890ab`
* Restore brigades
  * Example: `/opt/vg-dc-snaps/restoresnaps -r ab12cd34-5678-90ab-cdef-1234567890ab -f 4.33.222.0-migr-plan-20000101-000000.json > tee log-restore-4.33.222.0-migr-plan-20000101-000000-$(date +"%Y%m%d%H%M").log`
* Check the restore log for errors
  
## STEP 8: Switch the brigades instances to the new ones

* Use `vgmigr` datacenter account to switch the brigades instances to the new ones
* Switch the brigades instances
  * Example: `/opt/vg-dc-snaps/switch_local_migr.sh switch -r ab12cd34-5678-90ab-cdef-1234567890ab -f 4.33.222.0-migr-prepared-20000101-000000.json`
* Use `vgvpnapi` datacenter account to sync subdomains
* Sync subdomains with subdomain management API
  * Example: `SSH_KEY=~/.ssh/id_ed25519 /opt/vg-dc-vpnapi/delegation-sync.sh`

## STEP 9: Wait for the DNS propagation

* Wait for the DNS propagation 24 hours

## STEP 10: Clean up

* Use `vgmigr` datacenter account to clean up
* Clean up the old brigades instances
  * Example: `/opt/vg-dc-snaps/switch_local_migr.sh delete -r ab12cd34-5678-90ab-cdef-1234567890ab -f 4.33.222.0-migr-prepared-20000101-000000.json`
* Clean up the reservation
  * Example: `/opt/vg-dc-snaps/create_reservation.sh delete -f ab12cd34-5678-90ab-cdef-1234567890ab`