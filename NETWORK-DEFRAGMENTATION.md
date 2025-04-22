# Network defragmentation via migration via encrypted snapshots

This document describes the process of migrating a set of brigades within a single data center using encrypted snapshots aiming to defragment the network. The migration process involves creating a snapshot of the brigades, preparing the snapshot for migration, and then restoring the brigades with the migration plan.

## Required special configuration in the data center

* Well configured datacenter
* Datacenter private RSA key online (/etc)
* Authority private RSA key online (~/.secret)
* Master shuffler key online (~/.secret)

## STEP 1: Prepare database to avoid brigades manipulations

* What we need to do:
  * Mark endpoint_ipv4 addresses as not `enabled` in the database
* How to do it:
  * Use `vgadmin` datacenter account
  * Use `/home/vgadmin/01-admin-defragnet-prepare.sh` helper script

## STEP 2: Migrate brigades

* What we need to do:
  * Create a snapshot
  * Create a reservation
  * Prepare the snapshot for migration
  * Create a migration plan
  * Restore brigades with the migration plan
* How to do it:
  * Use `vgmigr` datacenter account
  * Use `/home/vgmigr/01-migr-defragnet-propagade.sh <external_network> <service_network>` helper script (use in `screen` or `tmux`)
  * Check logs in `/home/vgmigr/migr-logs/` directory (`202504212222-111.22.55.0-10.10.10.0-migr-plan-20250421-222057.log` style)

## STEP 3: Switch the brigades instances to the new ones

**CHECK LOGS BEFORE!!!**

* What we need to do:
  * Switch the brigades instances
  * Sync subdomains with subdomain management API
* How to do it:
  * Use `vgmigr` datacenter account
  * Use `/home/vgmigr/02-migr-defragnet-switch.sh <external_network> <service_network>` helper script

## STEP 4: Wait for the DNS propagation

* Wait for the DNS propagation one hour or more

## STEP 5: Clean up brigades instances

* What we need to do:
  * Clean up the old brigades instances
  * Clean up the reservation
  * Remove auxiliary files (in `/home/vgmigr/tmp/` directory)
* How to do it:
        * Use `vgmigr` datacenter account
        * Use `/home/vgmigr/03-migr-defragnet-cleanup.sh <external_network> <service_network>` helper script

## STEP 6: Cleanup address space and pairs

* What we need to do:
  * Cleanup address space
  * Cleanup pairs
* How to do it:
  * Use `vgadmin` datacenter account
  * Use `/home/vgadmin/02-admin-defragnet-cleanup.sh <external_network> <service_network>` helper script
