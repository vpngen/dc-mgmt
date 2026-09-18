BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '041-endpoint-reachability-provenance', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen', '032-socket-gen-fix', '033-socket-gen-fix', '034-isolation-groups', '035-stats-add-lastseen', '036-stats-add-instance_created_at', '037-replications', '038-stats-protection', '039-stats-migr-carryforward-grant', '040-endpoint-reachability']);

-- Provenance for each day's verdicts.
--
-- The rotation rules destroy things on the strength of these rows -- a parked
-- probe reclaimed after three clean days cannot be brought back, and the
-- address is re-enabled for organic placement at the same time. When a decision
-- later looks wrong, the first question is "what was that verdict based on?",
-- and today there is no way to answer it.
--
-- Neither column is read by any rule. They exist to make the history
-- self-explaining after the fact.

-- When the monitor produced the response. Note this is request time, not probe
-- time -- it is always ~now and is NOT a freshness signal. Recorded for audit
-- only, so a row can be tied back to a specific fetch.
ALTER TABLE :"schema_stats_name".endpoint_reachability
        ADD COLUMN generated_at timestamp without time zone NULL;

-- Median age of the individual node readings behind that day's verdicts. THIS
-- is the freshness signal: the monitor needs ~12h to work through a freshly
-- published list, and the ingest refuses to run when this exceeds
-- MAX_PROBE_AGE_H. Storing it means a surprising streak can be checked against
-- how fresh the underlying probing actually was -- a day that squeaked in just
-- under the limit is weaker evidence than one built on hour-old readings.
ALTER TABLE :"schema_stats_name".endpoint_reachability
        ADD COLUMN median_probe_age_s int NULL;

-- Rows written before this patch carry NULL in both, which is correct: we do
-- not know their provenance and should not pretend otherwise.

COMMIT;
