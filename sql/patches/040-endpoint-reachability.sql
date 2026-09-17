BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '040-endpoint-reachability', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen', '032-socket-gen-fix', '033-socket-gen-fix', '034-isolation-groups', '035-stats-add-lastseen', '036-stats-add-instance_created_at', '037-replications', '038-stats-protection', '039-stats-migr-carryforward-grant']);

-- Daily record of whether each endpoint address is reachable from inside Russia,
-- as judged by the external monitor we feed with endpoints-feed.sh.
--
-- Why this has to be stored rather than queried live: every rotation decision is
-- about a STREAK, not a reading. Destroying a parked probe is irreversible and
-- re-enables the address for organic placement, so it must only happen after the
-- address has looked clean for several consecutive days -- a single API call
-- cannot answer that. A probe gap on one day would otherwise look identical to
-- a lifted ban.
--
-- One row per address per day. Re-running the ingest on the same day overwrites
-- that day's row rather than adding a second, so the job is idempotent.
CREATE TABLE :"schema_stats_name".endpoint_reachability (
        endpoint_ipv4   inet    NOT NULL,
        observed_on     date    NOT NULL DEFAULT ((now() AT TIME ZONE 'UTC')::date),

        -- ok           - absent from the monitor's problem list, i.e. reachable
        -- blocked_ru   - answers abroad, not from Russia
        -- partial      - some Russian probes get through, some do not
        -- no_listener  - nothing answering anywhere; says nothing about blocking
        -- unmonitored  - address is not in the monitor's watch set
        verdict         text    NOT NULL,

        -- 'reachable' means a foreign vantage point reached it, which is what
        -- separates blocked_ru from no_listener.
        reference       text,

        ru_reachable    int,
        ru_unreachable  int,
        ru_no_data      int,
        ru_total        int,

        -- Our own liveness probe (nc -z <ip> 80) from outside Russia, recorded
        -- separately from the monitor's opinion. blocked_ru with nc80_ok = false
        -- is not a blocking problem -- the endpoint itself is down, and migrating
        -- the brigade would carry the fault along with it.
        nc80_ok         boolean,

        observed_at     timestamp without time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),

        PRIMARY KEY (endpoint_ipv4, observed_on)
);

CREATE INDEX endpoint_reachability_observed_on_idx
        ON :"schema_stats_name".endpoint_reachability (observed_on);
CREATE INDEX endpoint_reachability_verdict_idx
        ON :"schema_stats_name".endpoint_reachability (verdict, observed_on);

-- Rolling 3-day summary, which is the window both rotation rules are written
-- against:
--
--   reclaim a parked probe   -> ok_days = 3 AND days = 3
--   migrate a blocked main   -> blocked_days >= 2
--
-- days < 3 means we simply have not watched the address long enough yet; treat
-- that as "not eligible" rather than as a negative result.
--
-- ONLY 'blocked_ru' counts as blocked. 'partial' is never actionable in either
-- direction: it is not proof of a block, so it must not trigger a migration, and
-- it is not proof of a clear address, so it must not allow a probe to be
-- destroyed. It breaks a clean streak by construction -- ok_days counts only
-- 'ok', so a partial day gives ok_days < days and the address falls out of the
-- reclaim set. partial_days is exposed for visibility, not for decisions.
CREATE VIEW :"schema_stats_name".endpoint_reachability_recent AS
SELECT  endpoint_ipv4,
        count(*)                                            AS days,
        count(*) FILTER (WHERE verdict = 'ok')              AS ok_days,
        count(*) FILTER (WHERE verdict = 'blocked_ru')      AS blocked_days,
        count(*) FILTER (WHERE verdict = 'partial')         AS partial_days,
        count(*) FILTER (WHERE verdict = 'no_listener')     AS no_listener_days,
        max(observed_on)                                    AS last_seen_on,
        (array_agg(verdict  ORDER BY observed_on DESC))[1]  AS latest_verdict,
        (array_agg(nc80_ok  ORDER BY observed_on DESC))[1]  AS latest_nc80_ok
FROM    :"schema_stats_name".endpoint_reachability
WHERE   observed_on > ((now() AT TIME ZONE 'UTC')::date - 3)
GROUP BY endpoint_ipv4;

-- The ingest job runs as the stats role, alongside endpoints-feed.sh.
GRANT SELECT, INSERT, UPDATE, DELETE
        ON :"schema_stats_name".endpoint_reachability TO :"stats_dbuser";
GRANT SELECT
        ON :"schema_stats_name".endpoint_reachability_recent TO :"stats_dbuser";

-- The rotation executor runs as the migration role and only ever reads verdicts.
GRANT SELECT
        ON :"schema_stats_name".endpoint_reachability TO :"migr_dbuser";
GRANT SELECT
        ON :"schema_stats_name".endpoint_reachability_recent TO :"migr_dbuser";

GRANT SELECT
        ON :"schema_stats_name".endpoint_reachability TO :"brigades_dbuser";
GRANT SELECT
        ON :"schema_stats_name".endpoint_reachability_recent TO :"brigades_dbuser";

COMMIT;
