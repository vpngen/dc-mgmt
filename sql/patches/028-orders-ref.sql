BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '028-orders-ref', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones']);

ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS is_registering;
ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS registering_started_at;
ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS is_processing;
ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS processing_started_at;
ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS is_filling;
ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS filling_started_at;
ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS is_completed;
ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS is_error;
ALTER TABLE pairs.pair_orders DROP COLUMN IF EXISTS error_at;

ALTER TABLE pairs.pair_orders ADD COLUMN IF NOT EXISTS pair_completed_at timestamp without time zone DEFAULT NULL;
ALTER TABLE pairs.pair_orders ADD COLUMN IF NOT EXISTS brigade_completed_at timestamp without time zone DEFAULT NULL;
ALTER TABLE pairs.pair_orders ADD COLUMN IF NOT EXISTS failed_at timestamp without time zone DEFAULT NULL;

COMMIT;