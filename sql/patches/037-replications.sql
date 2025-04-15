BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '037-replications', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen', '032-socket-gen-fix', '033-socket-gen-fix', '034-isolation-groups', '035-stats-add-lastseen', '036-stats-add-instance_created_at']);

ALTER TABLE brigades.reservations ADD COLUMN filter text NOT NULL DEFAULT '';

CREATE TABLE brigades.replications (
    replication_id uuid PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
    src_filter text NOT NULL DEFAULT '',
    dst_filter text NOT NULL DEFAULT '',
    update_time timestamp with time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC')
);    

CREATE TABLE brigades.replicated_endpoints_ipv4 (
    endpoint_ipv4 inet NOT NULL,
    replication_id uuid NOT NULL,
    src_dst bool NOT NULL DEFAULT false, -- true if src, false if dst
    update_time timestamp with time zone NOT NULL DEFAULT (now() AT TIME ZONE 'UTC'),
    FOREIGN KEY (endpoint_ipv4) REFERENCES pairs.pairs_endpoints_ipv4 (endpoint_ipv4),
    FOREIGN KEY (replication_id) REFERENCES brigades.replications (replication_id),
    PRIMARY KEY (endpoint_ipv4, replication_id)
);

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
        LEFT JOIN brigades.replicated_endpoints_ipv4 AS re ON (re.endpoint_ipv4=pei.endpoint_ipv4 AND re.src_dst = false)
        LEFT JOIN brigades.brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
    WHERE
        p.is_active
    AND
        p.on_demand=FALSE
    AND
        pei.enabled
    AND 
        pei.endpoint_num = 0
    AND
        o.endpoint_ipv4 IS NULL
    AND
        r.endpoint_ipv4 IS NULL
    AND
        re.endpoint_ipv4 IS NULL
    GROUP BY p.pair_id
    HAVING
        COUNT(pei.*)-COUNT(b.*) > 0
;

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
        LEFT JOIN brigades.replicated_endpoints_ipv4 AS re ON (re.endpoint_ipv4=pei.endpoint_ipv4 AND re.src_dst = false)
        LEFT JOIN brigades.brigades AS b ON b.endpoint_ipv4=pei.endpoint_ipv4
        LEFT JOIN brigades.domains_endpoints_ipv4 AS dei ON dei.endpoint_ipv4 = pei.endpoint_ipv4
    WHERE
        pei.enabled
    AND
        o.endpoint_ipv4 IS NULL
    AND
        r.endpoint_ipv4 IS NULL
    AND
        re.endpoint_ipv4 IS NULL
    AND
        b.endpoint_ipv4 IS NULL
;

COMMIT;