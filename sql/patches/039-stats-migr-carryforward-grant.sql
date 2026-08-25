BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '039-stats-migr-carryforward-grant', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen', '032-socket-gen-fix', '033-socket-gen-fix', '034-isolation-groups', '035-stats-add-lastseen', '036-stats-add-instance_created_at', '037-replications', '038-stats-protection']);

-- Migration gives a brigade a new instance_id, so it starts a fresh
-- brigades_stats row with peak_active_users and protected_until at zero while
-- its real history stays on the parked instance. Cleanup then deletes that
-- parked brigade row, and the ON DELETE CASCADE from 012-brigades-instance
-- takes the history with it -- leaving a busy brigade looking empty to
-- getwasted, which is exactly what 038 was added to prevent.
--
-- The cleanup scripts therefore carry those columns onto the new main before
-- deleting the old instance. They run as the migration role, which 021 gave
-- only SELECT/INSERT/DELETE on this schema, so the UPDATE fails.
--
-- Column-level grant rather than a table-level one: the migration role must not
-- be able to rewrite active_users_count or the traffic totals, which collectstats
-- owns and reassigns from node state on every run.

GRANT UPDATE (
        peak_active_users,
        peak_active_users_at,
        protected_until
) ON :"schema_stats_name".brigades_stats TO :"migr_dbuser";

COMMIT;
