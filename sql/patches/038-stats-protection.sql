BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '038-stats-protection', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen', '032-socket-gen-fix', '033-socket-gen-fix', '034-isolation-groups', '035-stats-add-lastseen', '036-stats-add-instance_created_at', '037-replications']);

-- High-water mark of active_users_count (users online in the last 30 days).
-- active_users_count itself is overwritten in place by collectstats on every
-- run, so a brigade that was popular last month is indistinguishable from one
-- that never had a single user. These columns keep that history.
ALTER TABLE stats.brigades_stats ADD COLUMN peak_active_users int NOT NULL DEFAULT 0;

-- Last time active_users_count was at or above the protection threshold.
ALTER TABLE stats.brigades_stats ADD COLUMN peak_active_users_at timestamp without time zone NULL;

-- Brigades that reached the threshold are exempt from getwasted's inactive
-- sweep until this deadline. Set by collectstats, moves forward only.
ALTER TABLE stats.brigades_stats ADD COLUMN protected_until timestamp without time zone NULL;

CREATE INDEX brigades_stats_protected_until_idx ON stats.brigades_stats (protected_until);

COMMIT;
