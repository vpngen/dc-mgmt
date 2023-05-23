BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '004-stats' , ARRAY[ '001-init', '002-roles', '003-stats']);

ALTER TABLE :"schema_stats_name".brigades_stats RENAME COLUMN last_visit TO first_visit;
ALTER TABLE :"schema_stats_name".brigades_statistics RENAME COLUMN last_visit TO first_visit;

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"stats_dbuser";

COMMIT;