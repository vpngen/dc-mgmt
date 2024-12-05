BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '030-viewsfixes', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers']);


-- The view that lists all the available IP address 
-- slots (endpoint IPv4 addresses) for each pair in the pairs table 
-- that are not yet assigned in the brigades table.
DROP VIEW IF EXISTS brigades.slots;
CREATE VIEW brigades.slots AS 
    SELECT
        p.pair_id,
        p.control_ip,
        pei.endpoint_ipv4,
        dei.domain_name
    FROM 
        pairs.pairs AS p
        JOIN pairs.pairs_endpoints_ipv4 AS pei ON pei.pair_id=p.pair_id
        LEFT JOIN brigades.orphaned_endpoints_ipv4 AS o ON o.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN brigades.reserved_endpoints_ipv4 AS r ON r.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN brigades.brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN brigades.domains_endpoints_ipv4 AS dei ON dei.endpoint_ipv4 = pei.endpoint_ipv4
    WHERE
        pei.enabled
    AND
        o.endpoint_ipv4 IS NULL
    AND
        r.endpoint_ipv4 IS NULL
    AND
        b.endpoint_ipv4 IS NULL
;

COMMIT;