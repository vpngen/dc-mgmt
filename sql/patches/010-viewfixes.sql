BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '010-viewfixes', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains']);

DROP VIEW IF EXISTS :"schema_pairs_name".private_cidr_nets_weight;
CREATE VIEW :"schema_pairs_name".private_cidr_nets_weight AS (
    SELECT
        private_cidr_nets.id,
        private_cidr_nets.ipv4_net,
        2^masklen(private_cidr_nets.ipv4_net) - COUNT(pairs.*) - 2 AS weight
    FROM
        :"schema_pairs_name".private_cidr_nets
        LEFT JOIN :"schema_pairs_name".pairs ON pairs.control_ip <<= private_cidr_nets.ipv4_net
    GROUP BY private_cidr_nets.ipv4_net
    HAVING 2^masklen(private_cidr_nets.ipv4_net) - COUNT(pairs.*) - 2 > 0
);

DROP VIEW IF EXISTS :"schema_brigades_name".ipv4_cgnat_nets_weight;
CREATE VIEW :"schema_brigades_name".ipv4_cgnat_nets_weight AS (
    SELECT
        ipv4_cgnat_nets.id,
        ipv4_cgnat_nets.ipv4_net,
        2^(24 - masklen(ipv4_cgnat_nets.ipv4_net)) - COUNT(brigades.*) AS weight 
    FROM
        :"schema_brigades_name".ipv4_cgnat_nets
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.ipv4_cgnat <<= ipv4_cgnat_nets.ipv4_net
    GROUP BY ipv4_cgnat_nets.ipv4_net
    HAVING 2^(24 - masklen(ipv4_cgnat_nets.ipv4_net)) - COUNT(brigades.*) > 0
);

DROP VIEW IF EXISTS :"schema_pairs_name".ipv4_nets_weight;
CREATE VIEW :"schema_pairs_name".ipv4_nets_weight AS (
    SELECT
        ipv4_nets.id,
        ipv4_nets.ipv4_net,
        ipv4_nets.gateway,
        2^masklen(ipv4_nets.ipv4_net) - COUNT(pairs_endpoints_ipv4.*) - 2 AS weight
    FROM
        :"schema_pairs_name".ipv4_nets
        LEFT JOIN :"schema_pairs_name".pairs_endpoints_ipv4 ON pairs_endpoints_ipv4.endpoint_ipv4 <<= ipv4_nets.ipv4_net
    GROUP BY ipv4_nets.ipv4_net
    HAVING 2^masklen(ipv4_nets.ipv4_net) - COUNT(pairs_endpoints_ipv4.*) - 2 > 0
);

DROP VIEW IF EXISTS :"schema_brigades_name".ipv6_ula_nets_iweight;
CREATE VIEW :"schema_brigades_name".ipv6_ula_nets_iweight AS (
    SELECT
        ipv6_ula_nets.id,
        ipv6_ula_nets.ipv6_net,
        COUNT(brigades.*) AS iweight 
    FROM
        :"schema_brigades_name".ipv6_ula_nets
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.ipv6_ula <<= ipv6_ula_nets.ipv6_net
    GROUP BY ipv6_ula_nets.ipv6_net
);

DROP VIEW IF EXISTS :"schema_brigades_name".ipv6_keydesk_nets_iweight;
CREATE VIEW :"schema_brigades_name".ipv6_keydesk_nets_iweight AS (
    SELECT
        ipv6_keydesk_nets.id,
        ipv6_keydesk_nets.ipv6_net,
        COUNT(brigades.*) AS iweight
    FROM
        :"schema_brigades_name".ipv6_keydesk_nets
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.keydesk_ipv6 <<= ipv6_keydesk_nets.ipv6_net
    GROUP BY ipv6_keydesk_nets.ipv6_net
);

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"brigades_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"brigades_dbuser";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"stats_dbuser";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"stats_dbuser";

COMMIT;