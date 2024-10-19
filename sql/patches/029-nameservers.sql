BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '029-nameservers', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref']);

ALTER TABLE brigades.domains_endpoints_ipv4 ADD COLUMN IF NOT EXISTS nameservers text NOT NULL;

DROP VIEW IF EXISTS brigades.meta_brigades;
CREATE VIEW brigades.meta_brigades AS 
        SELECT
                b.pair_id,
                b.brigade_id,
                b.instance_id,
                b.brigadier,
                b.endpoint_ipv4,
                b.domain_name,
                b.dns_ipv4,
                b.dns_ipv6,
                b.keydesk_ipv6,
                b.ipv4_cgnat,
                b.ipv6_ula,
                b.person,
                b.main,
                p.control_ip,
                d.nameservers
        FROM
                brigades.brigades AS b
        JOIN 
                pairs.pairs AS p ON p.pair_id=b.pair_id
        LEFT JOIN
                brigades.domains_endpoints_ipv4 AS d ON d.domain_name=b.domain_name
;


COMMIT;