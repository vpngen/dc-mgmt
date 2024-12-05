BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '033-socket-gen-fix', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen', '032-socket-gen-fix']);

ALTER TABLE pairs.pairs DROP COLUMN IF EXISTS endpoint_num;

CREATE TABLE IF NOT EXISTS pairs.endpoint_nums (
        endpoint_num integer NOT NULL,
        zone text NOT NULL,
        update_time timestamp without time zone NOT NULL,
        PRIMARY KEY (endpoint_num, zone)
);

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

ALTER TABLE pairs.pairs_endpoints_ipv4 ADD COLUMN IF NOT EXISTS endpoint_num INTEGER NOT NULL DEFAULT 0;

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
        pei.endpoint_num = 0
    AND
        o.endpoint_ipv4 IS NULL
    AND
        r.endpoint_ipv4 IS NULL
    GROUP BY p.pair_id
    HAVING
        COUNT(pei.*)-COUNT(b.*) > 0
;

COMMIT;