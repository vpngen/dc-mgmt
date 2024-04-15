BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '016-viewfixes', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats']);

-- The view that lists all the available IP address 
-- slots (endpoint IPv4 addresses) for each pair in the pairs table 
-- that are not yet assigned in the brigades table.
DROP VIEW IF EXISTS :"schema_brigades_name".slots;
CREATE VIEW :"schema_brigades_name".slots AS 
    SELECT
        p.pair_id,
        p.control_ip,
        pei.endpoint_ipv4,
        dei.domain_name
    FROM 
        :"schema_pairs_name".pairs AS p
        JOIN :"schema_pairs_name".pairs_endpoints_ipv4 AS pei ON pei.pair_id=p.pair_id
        LEFT JOIN :"schema_brigades_name".orphaned_endpoints_ipv4 AS o ON o.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"schema_brigades_name".reserved_endpoints_ipv4 AS r ON r.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"schema_brigades_name".brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN :"schema_pairs_name".domains_endpoints_ipv4 AS dei ON dei.endpoint_ipv4 = pei.endpoint_ipv4
    WHERE
        o.endpoint_ipv4 IS NULL
    AND
        r.endpoint_ipv4 IS NULL
    AND
        b.endpoint_ipv4 IS NULL
;

COMMIT;