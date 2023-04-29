BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '006-stats' , ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats']);

ALTER TABLE :"schema_stats_name".brigades_stats ADD CONSTRAINT brigade_id_unique UNIQUE(brigade_id);
ALTER TABLE :"schema_stats_name".brigades_stats ADD COLUMN IF NOT EXISTS throttled_users_count int NOT NULL DEFAULT 0;

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"stats_dbuser";

COMMIT;