BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '011-collectsnaps', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes']);

CREATE ROLE :"snaps_dbuser" WITH LOGIN;

GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"snaps_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"snaps_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_pairs_name" GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO "snaps_dbuser";

GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"snaps_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"snaps_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_brigades_name" GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO "snaps_dbuser";

CREATE ROLE :"migr_dbuser" WITH LOGIN;

GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"snaps_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"migr_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_pairs_name" GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO "migr_dbuser";

GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"snaps_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"migr_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_brigades_name" GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO "migr_dbuser";

COMMIT;