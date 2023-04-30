BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '007-stats' , ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats']);

ALTER TABLE :"schema_stats_name".brigades_stats ALTER COLUMN update_time SET DEFAULT now();

COMMIT;