BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '018-viewfixes', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes']);

DROP VIEW IF EXISTS :"schema_brigades_name".meta_brigades;
CREATE VIEW :"schema_brigades_name".meta_brigades AS 
    SELECT
        brigades.pair_id,
        brigades.brigade_id,
    	brigades.brigadier,
    	brigades.endpoint_ipv4,
        brigades.domain_name,
    	brigades.dns_ipv4,
    	brigades.dns_ipv6,
    	brigades.keydesk_ipv6,
    	brigades.ipv4_cgnat,
    	brigades.ipv6_ula,
    	brigades.person,
	pairs.control_ip,
        brigades.main
    FROM
        :"schema_brigades_name".brigades,
    JOIN:"schema_pairs_name".pairs ON pairs.pair_id=brigades.pair_id
;

COMMIT;