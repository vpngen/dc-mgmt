BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '021-vgmigr-role', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes']);

GRANT USAGE ON SCHEMA :"schema_stats_name" TO :"migr_dbuser";

GRANT SELECT,INSERT,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"migr_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_stats_name" GRANT SELECT,INSERT,DELETE ON TABLES TO :"migr_dbuser";

COMMIT;