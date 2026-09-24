BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '042-slot-reclaim', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen', '032-socket-gen-fix', '033-socket-gen-fix', '034-isolation-groups', '035-stats-add-lastseen', '036-stats-add-instance_created_at', '037-replications', '038-stats-protection', '039-stats-migr-carryforward-grant', '040-endpoint-reachability', '041-endpoint-reachability-provenance']);

-- Audit trail for freeing a slot held by a parked (main = false) instance.
--
-- Those instances exist to keep ports 80/443 answering on an address that the censor
-- blocked, so the external monitor can tell us when the ban lifts. Once it has,
-- the probe has done its job and the slot can go back into the pool.
--
-- Reclaiming is three steps against two systems -- destroy the instance on its
-- control node, delete the database rows, re-enable the endpoint -- and it is
-- irreversible: the instance cannot be brought back, and the address becomes
-- available to addbrigade the moment it is enabled. Any step can fail on its
-- own, so intent is written here BEFORE acting and each step recorded as it
-- completes. A half-finished reclaim is then visible instead of silent.
CREATE TABLE :"schema_stats_name".slot_reclaim_log (
        id              bigserial PRIMARY KEY,

        endpoint_ipv4   inet    NOT NULL,
        brigade_id      uuid    NOT NULL,
        instance_id     uuid    NOT NULL,
        control_ip      inet    NOT NULL,

        -- Why we believed the address was clear. Kept so a reclaim that later
        -- looks wrong can be judged against the evidence it was made on.
        clean_days      int,
        latest_verdict  text,

        planned_at      timestamp without time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),

        -- NULL until attempted, then true/false. All three false with an error
        -- set means nothing happened; a mix means a partial reclaim needing a
        -- human.
        destroyed_ok    boolean,
        db_deleted      boolean,
        endpoint_enabled boolean,

        finished_at     timestamp without time zone,
        error           text
);

CREATE INDEX slot_reclaim_log_endpoint_idx ON :"schema_stats_name".slot_reclaim_log (endpoint_ipv4);
CREATE INDEX slot_reclaim_log_planned_at_idx ON :"schema_stats_name".slot_reclaim_log (planned_at);

-- Rows where some step failed, i.e. everything a human still needs to finish.
CREATE VIEW :"schema_stats_name".slot_reclaim_unfinished AS
SELECT *
FROM :"schema_stats_name".slot_reclaim_log
WHERE finished_at IS NULL
   OR destroyed_ok IS NOT TRUE
   OR db_deleted IS NOT TRUE
   OR endpoint_enabled IS NOT TRUE;

GRANT SELECT, INSERT, UPDATE ON :"schema_stats_name".slot_reclaim_log TO :"migr_dbuser";
GRANT USAGE ON SEQUENCE :"schema_stats_name".slot_reclaim_log_id_seq TO :"migr_dbuser";
GRANT SELECT ON :"schema_stats_name".slot_reclaim_unfinished TO :"migr_dbuser";
GRANT SELECT ON :"schema_stats_name".slot_reclaim_log TO :"stats_dbuser";
GRANT SELECT ON :"schema_stats_name".slot_reclaim_unfinished TO :"stats_dbuser";

-- Re-enabling the freed address is the last step of a reclaim, and the script
-- runs as the migration role. 021 gave it nothing on the pairs schema, so
-- without this the loop stays half-manual: the script would have to print a
-- list for someone to enable by hand as vgadmin.
--
-- Column-level, deliberately. The migration role gets to flip one boolean on an
-- address it just freed. It still cannot change endpoint_num, move an address
-- between pairs, or add and remove endpoints -- same discipline as 039.
GRANT UPDATE (enabled) ON :"schema_pairs_name".pairs_endpoints_ipv4 TO :"migr_dbuser";
GRANT SELECT ON :"schema_pairs_name".pairs_endpoints_ipv4 TO :"migr_dbuser";

COMMIT;
