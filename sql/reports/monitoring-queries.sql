-- ============================================================================
-- VPNGen DC — monitoring & reporting query pack
--
-- Run any block on its own:  psql -d vgrealm -f sql/reports/monitoring-queries.sql
-- or copy a single query.    psql -d vgrealm -c "<query>"
--
-- Roles: read-only everywhere. vgmigr / vgstats / vgvpnapi can all run these.
--
-- Sections
--   A  MAU health .................. what the number is and where it sits
--   B  Deletion risk (getwasted) ... what is about to be destroyed
--   C  Blocking monitor       ........ blocked, new, recovered, by network
--   D  Slot capacity & reclaim ..... what we have, what we freed
--   E  Blocking x MAU .............. does blocking explain the drop
--   F  Data quality ................ can these numbers be trusted
--
-- Terminology
--   MAU               = sum(active_users_count); a user is "active" if they
--                       connected at least once in the LAST 30 DAYS.
--   keys issued       = total_users_count; does not decay.
--   blocked           = verdict 'blocked_ru' ONLY. 'partial' is never
--                       actionable in either direction.
-- ============================================================================


-- ============================================================================
-- A. MAU HEALTH
-- ============================================================================

-- A1. Headline numbers. The single row to put at the top of any report.
SELECT count(*)                                                          AS brigades,
       sum(s.total_users_count)                                          AS keys_issued,
       sum(s.active_users_count)                                         AS mau,
       round(avg(s.active_users_count), 2)                               AS avg_active_per_brigade,
       round(100.0 * sum(s.active_users_count)
                   / NULLIF(sum(s.total_users_count), 0), 1)             AS active_pct
FROM stats.brigades_stats s
JOIN brigades.brigades b
  ON b.brigade_id = s.brigade_id AND b.instance_id = s.instance_id
WHERE b.main;


-- A2. MAU by brigade age. Shows where the users actually live.
--     New brigades contribute almost nothing, so a creation spike cannot
--     offset losses in the 90d+ cohort.
SELECT CASE WHEN s.created_at > (now() AT TIME ZONE 'utc') - interval '7 days'  THEN '1. age 0-7d'
            WHEN s.created_at > (now() AT TIME ZONE 'utc') - interval '30 days' THEN '2. age 7-30d'
            WHEN s.created_at > (now() AT TIME ZONE 'utc') - interval '90 days' THEN '3. age 30-90d'
            ELSE                                                                     '4. age 90d+' END AS cohort,
       count(*)                                                          AS brigades,
       sum(s.active_users_count)                                         AS mau,
       sum(s.total_users_count)                                          AS keys_issued,
       round(avg(s.active_users_count), 2)                               AS avg_active,
       round(100.0 * sum(s.active_users_count)
                   / NULLIF(sum(s.total_users_count), 0), 1)             AS active_pct
FROM stats.brigades_stats s
JOIN brigades.brigades b
  ON b.brigade_id = s.brigade_id AND b.instance_id = s.instance_id
WHERE b.main
GROUP BY 1 ORDER BY 1;


-- A3. Distribution of brigades by active-user count, with the getwasted
--     threshold marked. Watch the 0-4 and 5-9 rows: mass moving down into
--     them is the leading indicator of brigade loss.
SELECT CASE WHEN s.active_users_count < 5  THEN '0-4   DELETABLE NOW'
            WHEN s.active_users_count < 10 THEN '5-9   one step away'
            WHEN s.active_users_count < 15 THEN '10-14'
            WHEN s.active_users_count < 20 THEN '15-19'
            WHEN s.active_users_count < 30 THEN '20-29 (below protection)'
            ELSE                                '30+   auto-protected' END AS band,
       count(*)                                                           AS brigades,
       sum(s.active_users_count)                                          AS mau,
       sum(s.total_users_count)                                           AS keys_at_stake
FROM stats.brigades_stats s
JOIN brigades.brigades b
  ON b.brigade_id = s.brigade_id AND b.instance_id = s.instance_id
WHERE b.main
GROUP BY 1 ORDER BY 1;


-- A4. Protection coverage. collectstats only protects brigades seen with
--     >= 30 active users (PROTECTION_MIN_ACTIVE_USERS), for 2 months + 7 days.
--     Everything else is unprotected unless set manually.
SELECT count(*)                                                                    AS brigades,
       count(*) FILTER (WHERE s.protected_until > (now() AT TIME ZONE 'utc'))      AS protected,
       count(*) FILTER (WHERE s.protected_until IS NULL
                           OR s.protected_until <= (now() AT TIME ZONE 'utc'))     AS unprotected,
       round(100.0 * count(*) FILTER (WHERE s.protected_until > (now() AT TIME ZONE 'utc'))
                   / NULLIF(count(*), 0), 1)                                       AS protected_pct,
       min(s.protected_until) FILTER (WHERE s.protected_until > (now() AT TIME ZONE 'utc')) AS next_expiry
FROM stats.brigades_stats s
JOIN brigades.brigades b
  ON b.brigade_id = s.brigade_id AND b.instance_id = s.instance_id
WHERE b.main;


-- A5. Protection expiry schedule — when the shield lifts and how much is
--     exposed on each date.
SELECT s.protected_until::date                                           AS expires_on,
       count(*)                                                          AS brigades,
       sum(s.total_users_count)                                          AS keys_exposed
FROM stats.brigades_stats s
JOIN brigades.brigades b
  ON b.brigade_id = s.brigade_id AND b.instance_id = s.instance_id
WHERE b.main
  AND s.protected_until > (now() AT TIME ZONE 'utc')
GROUP BY 1 ORDER BY 1;


-- ============================================================================
-- B. DELETION RISK  (mirrors cmd/getwasted/main.go sqlGetInactive)
-- ============================================================================

-- B1. Exactly what getwasted would delete on its next run.
--     NOTE: joins on brigade_id only, matching getwasted's own query.
--     Age conditions are caller-supplied there, so this is an UPPER bound.
SELECT count(*)                                                          AS deletable_brigades,
       sum(bs.total_users_count)                                         AS keys_destroyed,
       sum(bs.active_users_count)                                        AS mau_lost
FROM stats.brigades_stats bs
JOIN brigades.brigades b            ON bs.brigade_id = b.brigade_id
LEFT JOIN brigades.brigades b2      ON b.brigade_id = b2.brigade_id AND b2.main = false
LEFT JOIN brigades.reserved_endpoints_ipv4 rei ON b.endpoint_ipv4 = rei.endpoint_ipv4
WHERE bs.active_users_count < 5
  AND (bs.protected_until IS NULL OR bs.protected_until <= (now() AT TIME ZONE 'utc'))
  AND b.main = true
  AND rei.endpoint_ipv4 IS NULL
  AND b2.brigade_id IS NULL;


-- B2. The same list, itemised — for reviewing before a sweep.
SELECT b.domain_name,
       host(b.endpoint_ipv4)                                             AS endpoint,
       host(p.control_ip)                                                AS control_node,
       bs.active_users_count                                             AS active,
       bs.total_users_count                                              AS keys,
       bs.created_at::date                                               AS created,
       bs.instance_created_at::date                                      AS instance_since,
       bs.update_time                                                    AS stats_fresh_at
FROM stats.brigades_stats bs
JOIN brigades.brigades b            ON bs.brigade_id = b.brigade_id
JOIN pairs.pairs p                  ON p.pair_id = b.pair_id
LEFT JOIN brigades.brigades b2      ON b.brigade_id = b2.brigade_id AND b2.main = false
LEFT JOIN brigades.reserved_endpoints_ipv4 rei ON b.endpoint_ipv4 = rei.endpoint_ipv4
WHERE bs.active_users_count < 5
  AND (bs.protected_until IS NULL OR bs.protected_until <= (now() AT TIME ZONE 'utc'))
  AND b.main = true
  AND rei.endpoint_ipv4 IS NULL
  AND b2.brigade_id IS NULL
ORDER BY bs.total_users_count DESC
LIMIT 100;


-- B3. Are deletion candidates concentrated on blocked addresses?
--     If yes, the cleanup is destroying block casualties rather than
--     abandoned brigades. THIS IS THE KEY QUERY FOR KOSTYA.
SELECT COALESCE(r.latest_verdict, 'not_monitored')                       AS verdict,
       count(*)                                                          AS deletable_brigades,
       sum(bs.total_users_count)                                         AS keys_at_stake
FROM stats.brigades_stats bs
JOIN brigades.brigades b            ON bs.brigade_id = b.brigade_id
LEFT JOIN brigades.brigades b2      ON b.brigade_id = b2.brigade_id AND b2.main = false
LEFT JOIN brigades.reserved_endpoints_ipv4 rei ON b.endpoint_ipv4 = rei.endpoint_ipv4
LEFT JOIN stats.endpoint_reachability_recent r ON r.endpoint_ipv4 = b.endpoint_ipv4
WHERE bs.active_users_count < 5
  AND (bs.protected_until IS NULL OR bs.protected_until <= (now() AT TIME ZONE 'utc'))
  AND b.main = true
  AND rei.endpoint_ipv4 IS NULL
  AND b2.brigade_id IS NULL
GROUP BY 1 ORDER BY 2 DESC;


-- ============================================================================
-- C. BLOCKING MONITOR
-- ============================================================================

-- C1. Daily verdict history. The top-line blocking trend.
SELECT observed_on,
       count(*)                                          AS addresses_observed,
       count(*) FILTER (WHERE verdict = 'ok')            AS ok,
       count(*) FILTER (WHERE verdict = 'blocked_ru')    AS blocked_ru,
       count(*) FILTER (WHERE verdict = 'partial')       AS partial,
       count(*) FILTER (WHERE verdict = 'no_listener')   AS no_listener,
       count(*) FILTER (WHERE verdict = 'unmonitored')   AS unmonitored
FROM stats.endpoint_reachability
GROUP BY 1 ORDER BY 1 DESC LIMIT 30;


-- C2. THE MIGRATION QUEUE — blocked addresses carrying a live main brigade,
--     blocked 2+ consecutive days. Ordered by users at risk.
--     nc80_ok = false means the endpoint itself is down, NOT a block; those
--     must not be migrated (the fault travels with the brigade).
SELECT host(r.endpoint_ipv4)                             AS endpoint,
       r.blocked_days,
       r.latest_verdict,
       r.latest_nc80_ok,
       b.domain_name,
       s.active_users_count                              AS active_users,
       s.total_users_count                               AS keys,
       host(p.control_ip)                                AS control_node
FROM stats.endpoint_reachability_recent r
JOIN brigades.brigades b   ON b.endpoint_ipv4 = r.endpoint_ipv4 AND b.main
JOIN stats.brigades_stats s ON s.brigade_id = b.brigade_id AND s.instance_id = b.instance_id
JOIN pairs.pairs p         ON p.pair_id = b.pair_id
WHERE r.blocked_days >= 2
ORDER BY s.active_users_count DESC;


-- C3. Blocking pressure by /24. Where blocking is actually hitting — the map
--     for deciding which ranges to stop buying from.
SELECT network(set_masklen(endpoint_ipv4::inet, 24))     AS net_24,
       count(*)                                          AS addresses,
       count(*) FILTER (WHERE verdict = 'blocked_ru')    AS blocked,
       round(100.0 * count(*) FILTER (WHERE verdict = 'blocked_ru')
                   / NULLIF(count(*), 0), 1)             AS blocked_pct
FROM stats.endpoint_reachability
WHERE observed_on = (SELECT max(observed_on) FROM stats.endpoint_reachability)
GROUP BY 1
HAVING count(*) FILTER (WHERE verdict = 'blocked_ru') > 0
ORDER BY blocked DESC;


-- C4. NEW blocks on the latest day — addresses blocked today that were
--     never blocked before. The daily "what did we lose" number.
WITH latest AS (SELECT max(observed_on) AS d FROM stats.endpoint_reachability),
today AS (
    SELECT endpoint_ipv4 FROM stats.endpoint_reachability, latest
    WHERE observed_on = latest.d AND verdict = 'blocked_ru'),
before AS (
    SELECT DISTINCT endpoint_ipv4 FROM stats.endpoint_reachability, latest
    WHERE observed_on < latest.d AND verdict = 'blocked_ru')
SELECT host(t.endpoint_ipv4)                             AS newly_blocked,
       b.domain_name,
       s.active_users_count                              AS active_users,
       s.total_users_count                               AS keys
FROM today t
LEFT JOIN before o USING (endpoint_ipv4)
LEFT JOIN brigades.brigades b ON b.endpoint_ipv4 = t.endpoint_ipv4 AND b.main
LEFT JOIN stats.brigades_stats s ON s.brigade_id = b.brigade_id AND s.instance_id = b.instance_id
WHERE o.endpoint_ipv4 IS NULL
ORDER BY s.active_users_count DESC NULLS LAST;


-- C5. RECOVERED addresses — previously blocked, clean on the latest day.
--     Evidence that a ban was lifted, and the input to reclaim decisions.
SELECT host(r.endpoint_ipv4)                             AS endpoint,
       r.ok_days, r.blocked_days, r.days, r.latest_verdict,
       (SELECT min(observed_on) FROM stats.endpoint_reachability e
         WHERE e.endpoint_ipv4 = r.endpoint_ipv4 AND e.verdict = 'blocked_ru') AS first_blocked_on
FROM stats.endpoint_reachability_recent r
WHERE r.latest_verdict = 'ok'
  AND EXISTS (SELECT 1 FROM stats.endpoint_reachability e
               WHERE e.endpoint_ipv4 = r.endpoint_ipv4 AND e.verdict = 'blocked_ru')
ORDER BY r.ok_days DESC;


-- C6. RECLAIM-ELIGIBLE — parked probes on addresses clean for 3 straight
--     days. These are the slots reclaim-slots.sh would free.
SELECT host(r.endpoint_ipv4)                             AS endpoint,
       r.days, r.ok_days, r.latest_verdict,
       b.domain_name,
       b.main                                            AS is_main,
       host(p.control_ip)                                AS control_node
FROM stats.endpoint_reachability_recent r
JOIN brigades.brigades b ON b.endpoint_ipv4 = r.endpoint_ipv4
JOIN pairs.pairs p       ON p.pair_id = b.pair_id
WHERE r.days = 3 AND r.ok_days = 3
  AND b.main = false                                     -- parked probe only
ORDER BY r.endpoint_ipv4;


-- ============================================================================
-- D. SLOT CAPACITY & RECLAIM
-- ============================================================================

-- D1. Free slots by /24 — where we can actually place brigades today.
SELECT network(set_masklen(endpoint_ipv4::inet, 24))     AS net_24,
       count(*)                                          AS free_slots
FROM brigades.slots
GROUP BY 1 ORDER BY free_slots DESC;


-- D2. Capacity summary against demand.
SELECT (SELECT count(*) FROM brigades.slots)                             AS free_slots,
       (SELECT count(*) FROM brigades.brigades WHERE main)               AS live_brigades,
       (SELECT count(*) FROM brigades.orphaned_endpoints_ipv4)           AS orphaned,
       (SELECT count(*) FROM brigades.reserved_endpoints_ipv4)           AS reserved,
       (SELECT count(*) FROM pairs.pairs_endpoints_ipv4 WHERE NOT enabled) AS disabled;


-- D3. Reclaim activity — slots recovered by the rotation loop.
SELECT planned_at::date                                  AS day,
       count(*)                                          AS attempted,
       count(*) FILTER (WHERE destroyed_ok AND db_deleted AND endpoint_enabled) AS fully_ok,
       count(*) FILTER (WHERE error IS NOT NULL)         AS errors
FROM stats.slot_reclaim_log
GROUP BY 1 ORDER BY 1 DESC LIMIT 30;


-- D4. Reclaims needing a human — partial or failed.
SELECT id, host(endpoint_ipv4) AS endpoint, host(control_ip) AS control_node,
       clean_days, latest_verdict,
       destroyed_ok, db_deleted, endpoint_enabled,
       planned_at, finished_at, error
FROM stats.slot_reclaim_unfinished
ORDER BY planned_at DESC;


-- ============================================================================
-- E. BLOCKING x MAU  — does blocking explain the drop?
-- ============================================================================

-- E1. Activity on blocked vs clean endpoints.
--     CAUTION reading this: active_users_count is a 30-DAY window, so a
--     freshly blocked brigade still shows its pre-block count. A gap only
--     becomes visible ~2-4 weeks after the block. Compare the SAME cohort
--     over time rather than blocked vs ok on one day.
SELECT COALESCE(r.latest_verdict, 'not_monitored')       AS verdict,
       count(*)                                          AS brigades,
       sum(s.active_users_count)                         AS mau,
       round(avg(s.active_users_count), 2)               AS avg_active,
       round(100.0 * sum(s.active_users_count)
                   / NULLIF(sum(s.total_users_count), 0), 1) AS active_pct
FROM brigades.brigades b
JOIN stats.brigades_stats s ON s.brigade_id = b.brigade_id AND s.instance_id = b.instance_id
LEFT JOIN stats.endpoint_reachability_recent r ON r.endpoint_ipv4 = b.endpoint_ipv4
WHERE b.main
GROUP BY 1 ORDER BY brigades DESC;


-- E2. THE DECAY TEST. Track this weekly. Brigades blocked today should lose
--     activity over the following 30 days while clean ones hold steady.
--     Record the output each week; the divergence (or its absence) is the
--     proof either way.
SELECT CASE WHEN r.blocked_days >= 2 THEN 'blocked_2d+'
            WHEN r.latest_verdict = 'ok' THEN 'clean'
            ELSE COALESCE(r.latest_verdict, 'not_monitored') END         AS group_,
       count(*)                                          AS brigades,
       round(avg(s.active_users_count), 2)               AS avg_active,
       round(avg(s.total_users_count), 2)                AS avg_keys,
       round(100.0 * sum(s.active_users_count)
                   / NULLIF(sum(s.total_users_count), 0), 1) AS active_pct
FROM brigades.brigades b
JOIN stats.brigades_stats s ON s.brigade_id = b.brigade_id AND s.instance_id = b.instance_id
LEFT JOIN stats.endpoint_reachability_recent r ON r.endpoint_ipv4 = b.endpoint_ipv4
WHERE b.main
GROUP BY 1 ORDER BY 1;


-- E3. Migration exposure — brigades migrated recently vs never, among old
--     brigades. Migrated ones were usually blocked first, so a lower
--     active_pct here is the footprint of the dark period.
--     SURVIVORSHIP WARNING: brigades deleted after a block are absent from
--     this query entirely. It measures survivors only and CANNOT rule the
--     blocking theory out.
SELECT CASE WHEN s.instance_created_at > (now() AT TIME ZONE 'utc') - interval '30 days' THEN 'migrated 0-30d'
            WHEN s.instance_created_at > (now() AT TIME ZONE 'utc') - interval '60 days' THEN 'migrated 30-60d'
            WHEN s.instance_created_at > (now() AT TIME ZONE 'utc') - interval '90 days' THEN 'migrated 60-90d'
            ELSE 'not migrated in 90d' END               AS cohort,
       count(*)                                          AS brigades,
       sum(s.active_users_count)                         AS mau,
       round(avg(s.active_users_count), 2)               AS avg_active,
       round(avg(s.total_users_count), 2)                AS avg_keys,
       round(100.0 * sum(s.active_users_count)
                   / NULLIF(sum(s.total_users_count), 0), 1) AS active_pct
FROM brigades.brigades b
JOIN stats.brigades_stats s ON s.brigade_id = b.brigade_id AND s.instance_id = b.instance_id
WHERE b.main
  AND s.created_at < (now() AT TIME ZONE 'utc') - interval '90 days'
GROUP BY 1 ORDER BY 1;


-- ============================================================================
-- F. DATA QUALITY — can these numbers be trusted?
-- ============================================================================

-- F1. Statistics freshness per control node. Anything stale means its
--     brigades' user counts are FROZEN, not zero — they neither fall nor
--     rise, and getwasted correctly skips them.
SELECT host(p.control_ip)                                AS control_node,
       count(*)                                          AS brigades,
       max(s.update_time)                                AS last_stats,
       date_trunc('minute',
         (now() AT TIME ZONE 'utc') - max(s.update_time)) AS stale_for
FROM brigades.brigades b
JOIN pairs.pairs p ON p.pair_id = b.pair_id
LEFT JOIN stats.brigades_stats s ON s.brigade_id = b.brigade_id AND s.instance_id = b.instance_id
WHERE b.main
GROUP BY 1
HAVING max(s.update_time) < (now() AT TIME ZONE 'utc') - interval '6 hours'
    OR max(s.update_time) IS NULL
ORDER BY max(s.update_time) NULLS FIRST;


-- F2. Reachability ingest provenance. median_probe_age_s is the freshness
--     signal — the monitor needs ~12h to work through a published list, so
--     a day with a high median is weaker evidence than one with a low one.
SELECT observed_on,
       count(*)                                          AS rows_written,
       max(generated_at)                                 AS monitor_generated_at,
       round(avg(median_probe_age_s) / 3600.0, 1)        AS median_probe_age_h
FROM stats.endpoint_reachability
GROUP BY 1 ORDER BY 1 DESC LIMIT 14;


-- F3. Monitor coverage — how much of the fleet is actually being watched.
SELECT (SELECT count(*) FROM pairs.pairs_endpoints_ipv4)                 AS endpoints_registered,
       (SELECT count(DISTINCT endpoint_ipv4) FROM stats.endpoint_reachability
         WHERE observed_on = (SELECT max(observed_on) FROM stats.endpoint_reachability)) AS observed_latest_day,
       (SELECT count(*) FROM stats.endpoint_reachability
         WHERE observed_on = (SELECT max(observed_on) FROM stats.endpoint_reachability)
           AND verdict = 'unmonitored')                                  AS unmonitored;


-- ============================================================================
-- G. NETWORK-LEVEL VIEW  (added 2026-09-22)
-- ============================================================================

-- G1. Every /24 we own: how many addresses, how many carry a live brigade,
--     how many carry a parked probe, and how many are blocked today.
--     This is the inventory + blocking map in one row per network.
SELECT network(set_masklen(pei.endpoint_ipv4::inet, 24))                 AS net_24,
       count(*)                                                          AS addresses,
       count(*) FILTER (WHERE pei.enabled)                               AS enabled,
       count(b.brigade_id) FILTER (WHERE b.main)                         AS live_brigades,
       count(b.brigade_id) FILTER (WHERE NOT b.main)                     AS parked_probes,
       count(*) FILTER (WHERE er.verdict = 'blocked_ru')                 AS blocked,
       count(*) FILTER (WHERE er.verdict = 'blocked_ru' AND b.main)      AS blocked_with_brigade,
       round(100.0 * count(*) FILTER (WHERE er.verdict = 'blocked_ru')
                   / NULLIF(count(*), 0), 1)                             AS blocked_pct
FROM pairs.pairs_endpoints_ipv4 pei
LEFT JOIN brigades.brigades b  ON b.endpoint_ipv4 = pei.endpoint_ipv4
LEFT JOIN stats.endpoint_reachability er
       ON er.endpoint_ipv4 = pei.endpoint_ipv4
      AND er.observed_on = (SELECT max(observed_on) FROM stats.endpoint_reachability)
GROUP BY 1
ORDER BY blocked_pct DESC NULLS LAST;


-- G2. THE BASE MONITOR — first day of monitoring vs latest, per network.
--     Positive change = we are losing ground in that range.
--     Negative change = bans lifting, or brigades moved off.
--     Run this weekly; it is the trend Kostya's work is measured against.
WITH bounds AS (
    SELECT min(observed_on) AS first_day,
           max(observed_on) AS last_day
    FROM stats.endpoint_reachability
)
SELECT network(set_masklen(e.endpoint_ipv4::inet, 24))                   AS net_24,
       (SELECT first_day FROM bounds)                                    AS since,
       (SELECT last_day  FROM bounds)                                    AS until,
       count(*) FILTER (WHERE e.observed_on = bo.first_day
                          AND e.verdict = 'blocked_ru')                  AS blocked_first,
       count(*) FILTER (WHERE e.observed_on = bo.last_day
                          AND e.verdict = 'blocked_ru')                  AS blocked_last,
       count(*) FILTER (WHERE e.observed_on = bo.last_day
                          AND e.verdict = 'blocked_ru')
     - count(*) FILTER (WHERE e.observed_on = bo.first_day
                          AND e.verdict = 'blocked_ru')                  AS change
FROM stats.endpoint_reachability e
CROSS JOIN bounds bo
WHERE e.observed_on IN (bo.first_day, bo.last_day)
GROUP BY 1
HAVING count(*) FILTER (WHERE e.observed_on = bo.first_day AND e.verdict = 'blocked_ru') > 0
    OR count(*) FILTER (WHERE e.observed_on = bo.last_day  AND e.verdict = 'blocked_ru') > 0
ORDER BY change DESC, blocked_last DESC;


-- G3. Age-controlled blocking impact. The raw blocked-vs-clean comparison is
--     confounded: blocked brigades are BIGGER (70.7 keys vs 40.1), and big old
--     brigades convert keys to active users at a lower rate regardless of
--     blocking. This restricts both groups to the same age band so the
--     comparison is fair.
SELECT CASE WHEN r.blocked_days >= 2 THEN 'blocked 2d+' ELSE 'clean' END  AS grp,
       CASE WHEN s.created_at > (now() AT TIME ZONE 'utc') - interval '90 days'
            THEN 'under 90d' ELSE '90d+' END                              AS age,
       count(*)                                                           AS brigades,
       round(avg(s.active_users_count), 2)                                AS avg_active,
       round(avg(s.total_users_count), 2)                                 AS avg_keys,
       round(100.0 * sum(s.active_users_count)
                   / NULLIF(sum(s.total_users_count), 0), 1)              AS active_pct
FROM brigades.brigades b
JOIN stats.brigades_stats s ON s.brigade_id = b.brigade_id AND s.instance_id = b.instance_id
LEFT JOIN stats.endpoint_reachability_recent r ON r.endpoint_ipv4 = b.endpoint_ipv4
WHERE b.main
  AND (r.blocked_days >= 2 OR r.latest_verdict = 'ok')
GROUP BY 1, 2
ORDER BY 2, 1;
