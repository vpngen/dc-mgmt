BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '015-stats', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps'.'012-brigades-instance','013-slot-flags'. '014-viewfixes']);

ALTER TABLE :"schema_stats_name".brigades_stats DROP COLUMN IF EXISTS active_wg_users_count; 
ALTER TABLE :"schema_stats_name".brigades_stats DROP COLUMN IF EXISTS active_ipsec_users_count;
ALTER TABLE :"schema_stats_name".brigades_stats DROP COLUMN IF EXISTS total_wg_traffic_rx;
ALTER TABLE :"schema_stats_name".brigades_stats DROP COLUMN IF EXISTS total_wg_traffic_tx;
ALTER TABLE :"schema_stats_name".brigades_stats DROP COLUMN IF EXISTS total_ipsec_traffic_rx;
ALTER TABLE :"schema_stats_name".brigades_stats DROP COLUMN IF EXISTS total_ipsec_traffic_tx;

COMMIT;