BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.try_register_patch( '003-stats' , ARRAY['001-init', '002-roles']);

-- The table is designed to store various statistics related to brigades. 
-- Each record in the table represents the statistics of a single brigade 
-- at a specific hour (aligned to the nearest hour). 
CREATE TABLE :"schema_stats_name".brigades_statistics (
        brigade_id                      uuid NOT NULL, -- Not a foreign key, because brigades may be deleted.
        last_visit                      timestamp without time zone DEFAULT NULL, -- NULL means that keydesk was never visited by brigadier.
        total_users_count               int NOT NULL,
        throttled_users_count           int NOT NULL, -- Users that are throttled by exeeding traffic limits.
        active_users_count              int NOT NULL, -- Users that are was online in last 30 days.
        active_wg_users_count           int NOT NULL, 
        active_ipsec_users_count        int NOT NULL,
        total_traffic_rx                bigint NOT NULL, -- Total traffic received by all users.
        total_traffic_tx                bigint NOT NULL, -- Total traffic sent by all users.
        total_wg_traffic_rx             bigint NOT NULL,
        total_wg_traffic_tx             bigint NOT NULL,
        total_ipsec_traffic_rx          bigint NOT NULL,
        total_ipsec_traffic_tx          bigint NOT NULL,
        counters_update_time            timestamp without time zone NOT NULL, -- Time when users counters and traffic were updated.
        stats_update_time               timestamp without time zone NOT NULL, -- Time when stats (mainly last_visit) were updated.
        update_time                     timestamp without time zone NOT NULL DEFAULT now(), -- Time when record was created.
        align_time                      timestamp without time zone GENERATED ALWAYS AS (date_trunc('hour', update_time)) STORED, -- Time when record was created, rounded to hour.
        PRIMARY KEY (brigade_id, align_time) -- align_time is needed for hourly aligning.
);

CREATE INDEX brigades_statistics_last_visit_idx ON :"schema_stats_name".brigades_statistics (last_visit);
CREATE INDEX brigades_statistics_active_users_count_idx ON :"schema_stats_name".brigades_statistics (active_users_count);
CREATE INDEX brigades_statistics_brigade_id_active_users_count_idx ON :"schema_stats_name".brigades_statistics (brigade_id, active_users_count);
CREATE INDEX brigades_statistics_throttled_users_count_idx ON :"schema_stats_name".brigades_statistics (throttled_users_count);
CREATE INDEX brigades_statistics_align_time_idx ON :"schema_stats_name".brigades_statistics (align_time);

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"stats_dbuser";

COMMIT;