BEGIN;

CREATE SCHEMA :"schema_stats_name";
CREATE ROLE :"stats_dbuser" WITH LOGIN;
GRANT ALL PRIVILEGES ON SCHEMA :"schema_stats_name" TO :"stats_dbuser";

CREATE ROLE :"ministry_stats_dbuser" WITH LOGIN;
GRANT USAGE ON SCHEMA :"schema_stats_name" TO :"ministry_stats_dbuser";
GRANT SELECT ON SCHEMA :"schema_stats_name" TO :"ministry_stats_dbuser";

CREATE TABLE :"schema_stats_name".brigades_stats (
    brigade_id          uuid NOT NULL,
    create_at           timestamp without time zone NOT NULL DEFAULT NOW(),
    last_visit          timestamp without time zone DEFAULT NULL,
    user_count          int NOT NULL DEFAULT 0,
    FOREIGN KEY (brigade_id) REFERENCES :"schema_brigades_name".brigades (brigade_id) ON DELETE CASCADE
);

COMMIT;