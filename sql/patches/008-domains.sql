BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '008-domains', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats']);

CREATE TABLE :"schema_pairs_name".domains_endpoints_ipv4 (
        domain_name text UNIQUE NOT NULL,
        endpoint_ipv4 inet UNIQUE NOT NULL,
        FOREIGN KEY (endpoint_ipv4) REFERENCES :"schema_pairs_name".pairs_endpoints_ipv4 (endpoint_ipv4)
);

ALTER TABLE :"schema_brigades_name".brigades
ADD COLUMN domain_name text,
ADD CONSTRAINT domain_name_fk
FOREIGN KEY (domain_name) REFERENCES :"schema_pairs_name".domains_endpoints_ipv4 (domain_name);


DROP VIEW IF EXISTS :"schema_brigades_name".slots;
CREATE VIEW :"schema_brigades_name".slots AS 
    SELECT
        pairs.pair_id,
        pairs.control_ip,
        pairs_endpoints_ipv4.endpoint_ipv4,
        domains_endpoints_ipv4.domain_name
    FROM 
        :"schema_pairs_name".pairs
        JOIN :"schema_pairs_name".pairs_endpoints_ipv4 ON pairs_endpoints_ipv4.pair_id = pairs.pair_id
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.endpoint_ipv4 = pairs_endpoints_ipv4.endpoint_ipv4
        LEFT JOIN :"schema_pairs_name".domains_endpoints_ipv4 ON domains_endpoints_ipv4.endpoint_ipv4 = pairs_endpoints_ipv4.endpoint_ipv4
    WHERE
        NOT EXISTS (
            SELECT
            FROM :"schema_pairs_name".pairs_endpoints_ipv4
            WHERE endpoint_ipv4 = brigades.endpoint_ipv4
        );

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
	pairs.control_ip
    FROM
        :"schema_brigades_name".brigades,
        :"schema_pairs_name".pairs
    WHERE
        pairs.pair_id=brigades.pair_id
;

GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"brigades_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"brigades_dbuser";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"stats_dbuser";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"stats_dbuser";

COMMIT;