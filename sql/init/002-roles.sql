BEGIN;

SELECT _v.assert_user_is_superuser();
SELECT _v.try_register_patch( '002-roles' , ARRAY['001-init']);

-- Create role for pairs.

CREATE ROLE :"pairs_dbuser" WITH LOGIN;

GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";

GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";

-- Create role for brigades.

CREATE ROLE :"brigades_dbuser" WITH LOGIN;

GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"brigades_dbuser";
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_brigades_name" TO :"brigades_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"brigades_dbuser";

GRANT USAGE ON SCHEMA :"schema_stats_name" TO :"brigades_dbuser";
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_stats_name" TO :"brigades_dbuser";
GRANT SELECT,INSERT,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"brigades_dbuser";

GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"brigades_dbuser";
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_pairs_name" TO :"brigades_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"brigades_dbuser";

-- Create role for stats dbuser.

CREATE ROLE :"stats_dbuser" WITH LOGIN;
GRANT USAGE ON SCHEMA :"schema_stats_name" TO :"stats_dbuser";
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_stats_name" TO :"stats_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"stats_dbuser";

GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"stats_dbuser";
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA :"schema_brigades_name"  TO :"stats_dbuser";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"stats_dbuser";

GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"stats_dbuser";
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA :"schema_pairs_name"  TO :"stats_dbuser";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"stats_dbuser";

COMMIT;
