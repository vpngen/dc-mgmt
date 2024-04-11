BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '013-slot-flags', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps'.'012-brigades-instance']);

ALTER TABLE :"schema_pairs_name".pairs_endpoints_ipv4 ADD COLUMN enabled NOT NULL DEFAULT true;
CREATE INDEX pairs_endpoints_ipv4_endpoint_ipv4_enabled_idx ON :"schema_pairs_name".pairs_endpoints_ipv4 (endpoint_ipv4, enabled);

CREATE TABLE :"schema_brigades_name".orphaned_endpoints_ipv4 (
    endpoint_ipv4 inet PRIMARY KEY,
    update_time timestamp with time zone NOT NULL DEFAULT now() AT TIME ZONE 'UTC',
    FOREIGN KEY (endpoint_ipv4) REFERENCES :"schema_pairs_name".pairs_endpoints_ipv4 (endpoint_ipv4)
);

CREATE TABLE :"schema_brigades_name".reservations (
    reservation_id uuid PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
    dismission bool NOT NULL DEFAULT false,
    update_time timestamp with time zone NOT NULL DEFAULT now() AT TIME ZONE 'UTC'
);    

CREATE TABLE :"schema_brigades_name".reserved_endpoints_ipv4 (
    endpoint_ipv4 inet NOT NULL,
    reservation_id uuid NOT NULL,
    update_time timestamp with time zone NOT NULL DEFAULT now() AT TIME ZONE 'UTC',
    FOREIGN KEY (endpoint_ipv4) REFERENCES :"schema_pairs_name".pairs_endpoints_ipv4 (endpoint_ipv4),
    FOREIGN KEY (reservation_id) REFERENCES :"schema_brigades_name".reservations (reservation_id),
    PRIMARY KEY (endpoint_ipv4, reservation_id)
);

GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";
GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_pairs_name" GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO :"pairs_dbuser";

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_brigades_name" GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO :"pairs_dbuser";

GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"brigades_dbuser";
GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"brigades_dbuser";
GRANT USAGE ON SCHEMA :"schema_stats_name" TO :"brigades_dbuser";

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"brigades_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_pairs_name" GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO :"brigades_dbuser";

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"brigades_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_brigades_name" GRANT SELECT,INSERT,UPDATE,DELETE ON TABLES TO :"brigades_dbuser";

GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_stats_name" TO :"brigades_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_stats_name" GRANT USAGE,SELECT,UPDATE ON SEQUENCES TO :"brigades_dbuser";

GRANT SELECT,INSERT,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"brigades_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_stats_name" GRANT SELECT,INSERT,DELETE ON TABLES TO :"brigades_dbuser";

GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"stats_dbuser";
GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"stats_dbuser";

GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"stats_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_pairs_name" GRANT SELECT ON TABLES TO :"stats_dbuser";

GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"stats_dbuser";
ALTER DEFAULT PRIVILEGES IN SCHEMA :"schema_brigades_name" GRANT SELECT ON TABLES TO :"stats_dbuser";

COMMIT;