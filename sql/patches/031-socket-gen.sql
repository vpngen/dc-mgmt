BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '031-socket-gen', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes']);

-- Add the 'zone' column and remove the current primary key
ALTER TABLE pairs.endpoint_nums DROP CONSTRAINT endpoint_nums_pkey CASCADE;

-- Remove the identity behavior from endpoint_num
ALTER TABLE pairs.endpoint_nums ALTER COLUMN endpoint_num DROP IDENTITY;

-- Add the 'zone' column
ALTER TABLE pairs.endpoint_nums ADD COLUMN IF NOT EXISTS zone TEXT NOT NULL DEFAULT '';
ALTER TABLE pairs.endpoint_num_links ADD COLUMN IF NOT EXISTS zone TEXT NOT NULL DEFAULT '';

UPDATE pairs.endpoint_num_links SET zone=pair_orders.zone FROM pairs.pair_orders WHERE pair_orders.endpoint_num=endpoint_num_links.endpoint_num and pair_orders.completed_at IS NOT NULL;
UPDATE pairs.endpoint_nums SET zone=endpoint_num_links.zone FROM pairs.endpoint_num_links WHERE endpoint_num_links.endpoint_num=endpoint_nums.endpoint_num;

-- Set a composite primary key
ALTER TABLE pairs.endpoint_nums ADD PRIMARY KEY (endpoint_num, zone);
ALTER TABLE pairs.endpoint_num_links ADD CONSTRAINT endpoint_num_links_endpoint_num_zone_pkey FOREIGN KEY (endpoint_num, zone) REFERENCES pairs.endpoint_nums(endpoint_num,zone);
ALTER TABLE pairs.pair_orders ADD  CONSTRAINT pair_orders_endpoint_num_zone_fkey FOREIGN KEY (endpoint_num, zone) REFERENCES pairs.endpoint_nums(endpoint_num,zone);

-- Create a trigger function to handle zone-specific sequences
CREATE OR REPLACE FUNCTION generate_zone_specific_serial()
RETURNS TRIGGER AS $$
DECLARE
  new_endpoint_num INTEGER;
BEGIN
  -- Determine the next number for the zone
  SELECT COALESCE(MAX(endpoint_num), 0) + 1
  INTO new_endpoint_num
  FROM pairs.endpoint_nums
  WHERE zone = NEW.zone;

  -- Assign the new number
  NEW.endpoint_num = new_endpoint_num;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Attach the trigger to the table
CREATE TRIGGER set_zone_specific_serial
BEFORE INSERT ON pairs.endpoint_nums
FOR EACH ROW
WHEN (NEW.endpoint_num IS NULL) -- Only generate if not explicitly set
EXECUTE FUNCTION generate_zone_specific_serial();


-- Add the 'on_demand' column to the pairs table
ALTER TABLE pairs.pairs ADD COLUMN IF NOT EXISTS on_demand boolean NOT NULL DEFAULT FALSE;

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
        LEFT JOIN pairs.endpoint_num_links AS pen ON pen.endpoint_ipv4=pei.endpoint_ipv4
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
        pen.endpoint_num IS NULL
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