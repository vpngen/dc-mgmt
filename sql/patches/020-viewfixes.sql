BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '020-viewfixes', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes','019-ctrl-keys']);

DROP VIEW IF EXISTS :"schema_brigades_name".meta_brigades;
CREATE VIEW :"schema_brigades_name".meta_brigades AS 
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
	p.control_ip
    FROM
        :"schema_brigades_name".brigades AS b
    JOIN :"schema_pairs_name".pairs AS p ON p.pair_id=b.pair_id
;

COMMIT;