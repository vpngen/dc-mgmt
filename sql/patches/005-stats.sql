BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '005-stats' , ARRAY[ '001-init', '002-roles','003-stats', '004-stats']);

ALTER TABLE :"schema_stats_name".brigades_stats RENAME COLUMN create_at TO created_at;
ALTER TABLE :"schema_stats_name".brigades_stats RENAME COLUMN user_count TO total_users_count;
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS active_users_count             int NOT NULL DEFAULT 0;
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS active_wg_users_count          int NOT NULL DEFAULT 0, 
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS active_ipsec_users_count       int NOT NULL DEFAULT 0,
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS total_traffic_rx               bigint NOT NULL DEFAULT 0, 
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS total_traffic_tx               bigint NOT NULL DEFAULT 0,
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS total_wg_traffic_rx            bigint NOT NULL DEFAULT 0,
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS total_wg_traffic_tx            bigint NOT NULL DEFAULT 0,
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS total_ipsec_traffic_rx         bigint NOT NULL DEFAULT 0,
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS total_ipsec_traffic_tx         bigint NOT NULL DEFAULT 0,
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS update_time                    timestamp without time zone NOT NULL,

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"stats_dbuser";

COMMIT;