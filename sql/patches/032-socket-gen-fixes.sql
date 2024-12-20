BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '032-socket-gen-fix', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen']);


-- The view calculates the number of available IP address 
-- slots (free_slots_count) for each active pair in the pairs table, 
-- considering the IP addresses already assigned in the brigades table 
-- and select only those pairs that have at least one.
DROP VIEW IF EXISTS brigades.active_pairs;
CREATE VIEW brigades.active_pairs AS 
    SELECT 
        p.pair_id,
        p.zone,
        COUNT(pei.*)-COUNT(b.*) AS free_slots_count
    FROM 
        pairs.pairs AS p
        JOIN pairs.pairs_endpoints_ipv4 AS pei ON pei.pair_id=p.pair_id
        LEFT JOIN brigades.orphaned_endpoints_ipv4 AS o ON o.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN brigades.reserved_endpoints_ipv4 AS r ON r.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN brigades.brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
    WHERE
        p.is_active
    AND
        p.on_demand=FALSE
    AND
        pei.enabled
    AND
        o.endpoint_ipv4 IS NULL
    AND
        r.endpoint_ipv4 IS NULL
    GROUP BY p.pair_id
    HAVING
        COUNT(pei.*)-COUNT(b.*) > 0
;

ALTER TABLE pairs.pairs ADD COLUMN IF NOT EXISTS endpoint_num INTEGER NOT NULL DEFAULT 0;
UPDATE pairs.pairs SET endpoint_num=endpoint_num_links.endpoint_num FROM pairs.pairs_endpoints_ipv4 JOIN pairs.endpoint_num_links ON pairs_endpoints_ipv4.endpoint_ipv4=endpoint_num_links.endpoint_ipv4 WHERE pairs_endpoints_ipv4.pair_id=pairs.pair_id;

-- Add the 'zone' column and remove the current primary key
ALTER TABLE pairs.endpoint_nums DROP CONSTRAINT endpoint_nums_pkey CASCADE;

-- Drop the tables
DROP TRIGGER IF EXISTS set_zone_specific_serial ON pairs.endpoint_nums;
DROP TABLE IF EXISTS pairs.endpoint_num_links;
DROP TABLE IF EXISTS pairs.endpoint_nums;

COMMIT;