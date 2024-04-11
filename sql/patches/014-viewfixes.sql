BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '014-viewfixes', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags']);

-- The view calculates the number of available IP address 
-- slots (free_slots_count) for each active pair in the pairs table, 
-- considering the IP addresses already assigned in the brigades table 
-- and select only those pairs that have at least one.
DROP VIEW IF EXISTS :"schema_brigades_name".active_pairs;
CREATE VIEW :"schema_brigades_name".active_pairs AS 
    SELECT 
        p.pair_id, 
        COUNT(pei.*)-COUNT(b.*) AS free_slots_count
    FROM 
        :"schema_pairs_name".pairs AS p
        JOIN :"schema_pairs_name".pairs_endpoints_ipv4 AS pei ON pei.pair_id=p.pair_id
        LEFT JOIN :"schema_brigades_name".orphaned_endpoints_ipv4 AS o ON o.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"schema_brigades_name".reserved_endpoints_ipv4 AS r ON r.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"schema_brigades_name".brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
    WHERE
        p.is_active
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

-- The view that lists all the available IP address 
-- slots (endpoint IPv4 addresses) for each pair in the pairs table 
-- that are not yet assigned in the brigades table.
DROP VIEW IF EXISTS :"schema_brigades_name".slots;
CREATE VIEW :"schema_brigades_name".slots AS 
    SELECT
        p.pair_id,
        p.control_ip,
        pei.endpoint_ipv4
    FROM 
        :"schema_pairs_name".pairs AS p
        JOIN :"schema_pairs_name".pairs_endpoints_ipv4 AS pei ON pei.pair_id=p.pair_id
        LEFT JOIN :"schema_brigades_name".orphaned_endpoints_ipv4 AS o ON o.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"schema_brigades_name".reserved_endpoints_ipv4 AS r ON r.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"schema_brigades_name".brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
    WHERE
        o.endpoint_ipv4 IS NULL
    AND
        r.endpoint_ipv4 IS NULL
    AND
        b.endpoint_ipv4 IS NULL
;

DROP VIEW IF EXISTS :"schema_brigades_name".meta_brigades;
CREATE VIEW :"schema_brigades_name".meta_brigades AS 
    SELECT
        b.pair_id,
        b.brigade_id,
        b.instance_id,
    	b.brigadier,
    	b.endpoint_ipv4,
    	b.dns_ipv4,
    	b.dns_ipv6,
    	b.keydesk_ipv6,
    	b.ipv4_cgnat,
    	b.ipv6_ula,
    	b.person,
	p.control_ip
    FROM
        :"schema_brigades_name".brigades AS b
    JOIN 
        :"schema_pairs_name".pairs AS p ON p.pair_id=b.pair_id
;

COMMIT;