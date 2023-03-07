BEGIN;

-- Goes to `public` schema.

CREATE DOMAIN uint4 AS int4 CHECK (value >= 0);
CREATE DOMAIN uint8 AS int8 CHECK (value >= 0);
CREATE DOMAIN asn as int4 CHECK (value >= 1);

CREATE DOMAIN inet_ipv4_endpoint AS inet CHECK (family(value) = 4 AND masklen(value) = 32);
CREATE DOMAIN inet_ipv6_endpoint AS inet CHECK (family(value) = 6 AND masklen(value) = 128);
CREATE DOMAIN cidr_ipv4 AS inet CHECK (family(value) = 4);

CREATE DOMAIN inet_private_endpoint AS inet CHECK (family(value) = 4 AND masklen(value) = 32 AND (value << inet '10.0.0.0/8' OR value << inet '172.16.0.0/12' OR value << inet '192.168.0.0/16'));
CREATE DOMAIN cidr_private AS cidr CHECK (family(value) = 4 AND (value << cidr '10.0.0.0/8' OR value << cidr '172.16.0.0/12' OR value << cidr '192.168.0.0/16'));

CREATE DOMAIN inet_cgnat_endpoint AS inet CHECK (family(value) = 4 AND value << inet '100.64.0.0/10' AND masklen(value) = 32);
CREATE DOMAIN cidr_cgnat AS cidr CHECK (family(value) = 4 AND value << cidr '100.64.0.0/10');
CREATE DOMAIN inet_cgnat_24 AS inet CHECK (family(value) = 4 AND value << cidr '100.64.0.0/10' AND masklen(value) = 24);

CREATE DOMAIN inet_ula_endpoint AS inet CHECK (family(value) = 6 AND value << inet 'fd00::/8' AND masklen(value) = 128);
CREATE DOMAIN cidr_ula AS cidr CHECK (family(value) = 6 AND value << cidr 'fd00::/8');
CREATE DOMAIN inet_ula_64 AS inet CHECK (family(value) = 6 AND value << cidr 'fd00::/8' AND masklen(value) = 64);

-- Goes to `meta` schema, information for brigade creation.

CREATE SCHEMA :"schema_pairs_name";
CREATE SCHEMA :"schema_brigades_name";

-- External assignet nets.
CREATE TABLE :"schema_pairs_name".ipv4_nets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    ipv4_net cidr_ipv4 PRIMARY KEY NOT NULL,
    gateway  inet_ipv4_endpoint CHECK (gateway << ipv4_net)
);

-- Internal nets for infra. Control points range.
CREATE TABLE :"schema_pairs_name".private_cidr_nets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    ipv4_net cidr_private PRIMARY KEY NOT NULL
);

-- CGNAT nets for clients.
CREATE TABLE :"schema_brigades_name".ipv4_cgnat_nets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    ipv4_net cidr_cgnat PRIMARY KEY NOT NULL
);

-- ULA nets for clients.
CREATE TABLE :"schema_brigades_name".ipv6_ula_nets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    ipv6_net cidr_ula PRIMARY KEY NOT NULL
);

-- Keydesk nets.
CREATE TABLE :"schema_brigades_name".ipv6_keydesk_nets (
    id uuid NOT NULL DEFAULT gen_random_uuid(),
    ipv6_net cidr_ula PRIMARY KEY NOT NULL
);

-- Virtual machines pairs.
CREATE TABLE :"schema_pairs_name".pairs (
    pair_id             uuid PRIMARY KEY NOT NULL,
    control_ip          inet UNIQUE NOT NULL,
    is_active           bool NOT NULL
);

CREATE TABLE :"schema_pairs_name".pairs_endpoints_ipv4 (
    pair_id            uuid NOT NULL,
    endpoint_ipv4      inet_ipv4_endpoint UNIQUE NOT NULL,
    FOREIGN KEY (pair_id) REFERENCES :"schema_pairs_name".pairs (pair_id)
);

CREATE TABLE :"schema_brigades_name".brigades (
    brigade_id          uuid PRIMARY KEY NOT NULL,
    pair_id             uuid NOT NULL,
    brigadier           text UNIQUE NOT NULL,
    endpoint_ipv4       inet_ipv4_endpoint UNIQUE NOT NULL, -- port is always 51820
    dns_ipv4            inet_ipv4_endpoint NOT NULL,
    dns_ipv6            inet_ipv6_endpoint NOT NULL,
    keydesk_ipv6        inet_ipv6_endpoint NOT NULL,
    ipv4_cgnat          inet_cgnat_24 NOT NULL,
    ipv6_ula            inet_ula_64 NOT NULL,
    person              json NOT NULL,
    FOREIGN KEY (pair_id) REFERENCES :"schema_pairs_name".pairs (pair_id),
    FOREIGN KEY (endpoint_ipv4) REFERENCES :"schema_pairs_name".pairs_endpoints_ipv4 (endpoint_ipv4)
);

CREATE VIEW :"schema_brigades_name".active_pairs AS 
    SELECT 
        pairs.pair_id, 
        COUNT(pairs_endpoints_ipv4.*)-COUNT(brigades.*) AS free_slots_count
    FROM 
        :"schema_pairs_name".pairs 
        JOIN :"schema_pairs_name".pairs_endpoints_ipv4 ON pairs_endpoints_ipv4.pair_id=pairs.pair_id
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.endpoint_ipv4=pairs_endpoints_ipv4.endpoint_ipv4
    WHERE
            pairs.is_active
    GROUP BY pairs.pair_id
    HAVING
        COUNT(pairs_endpoints_ipv4.*)-COUNT(brigades.*) > 0
;

CREATE VIEW :"schema_brigades_name".slots AS 
    SELECT
        pairs.pair_id,
        pairs.control_ip,
        pairs_endpoints_ipv4.endpoint_ipv4
    FROM 
        :"schema_pairs_name".pairs
        JOIN :"schema_pairs_name".pairs_endpoints_ipv4 ON pairs_endpoints_ipv4.pair_id=pairs.pair_id
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.endpoint_ipv4=pairs_endpoints_ipv4.endpoint_ipv4
    WHERE
        NOT EXISTS (
            SELECT
            FROM :"schema_pairs_name".pairs_endpoints_ipv4
            WHERE endpoint_ipv4=brigades.endpoint_ipv4
        )
;

CREATE VIEW :"schema_brigades_name".meta_brigades AS 
    SELECT
        brigades.pair_id,
        brigades.brigade_id,
    	brigades.brigadier,
    	brigades.endpoint_ipv4,
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

CREATE VIEW :"schema_pairs_name".ipv4_nets_weight AS (
    SELECT
        ipv4_nets.id,
        ipv4_nets.ipv4_net,
        ipv4_nets.gateway,
        2^masklen(ipv4_nets.ipv4_net) - COUNT(pairs_endpoints_ipv4.*) - 2 AS weight
    FROM
        :"schema_pairs_name".ipv4_nets
        LEFT JOIN :"schema_pairs_name".pairs_endpoints_ipv4 ON pairs_endpoints_ipv4.endpoint_ipv4 << ipv4_nets.ipv4_net
    GROUP BY ipv4_nets.ipv4_net
    HAVING 2^masklen(ipv4_nets.ipv4_net) - COUNT(pairs_endpoints_ipv4.*) - 2 > 0
);

CREATE VIEW :"schema_pairs_name".private_cidr_nets_weight AS (
    SELECT
        private_cidr_nets.id,
        private_cidr_nets.ipv4_net,
        2^masklen(private_cidr_nets.ipv4_net) - COUNT(pairs.*) - 2 AS weight
    FROM
        :"schema_pairs_name".private_cidr_nets
        LEFT JOIN :"schema_pairs_name".pairs ON pairs.control_ip << private_cidr_nets.ipv4_net
    GROUP BY private_cidr_nets.ipv4_net
    HAVING 2^masklen(private_cidr_nets.ipv4_net) - COUNT(pairs.*) - 2 > 0
);

CREATE VIEW :"schema_brigades_name".ipv4_cgnat_nets_weight AS (
    SELECT
        ipv4_cgnat_nets.id,
        ipv4_cgnat_nets.ipv4_net,
        2^(24 - masklen(ipv4_cgnat_nets.ipv4_net)) - COUNT(brigades.*) AS weight 
    FROM
        :"schema_brigades_name".ipv4_cgnat_nets
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.ipv4_cgnat << ipv4_cgnat_nets.ipv4_net
    GROUP BY ipv4_cgnat_nets.ipv4_net
    HAVING 2^(24 - masklen(ipv4_cgnat_nets.ipv4_net)) - COUNT(brigades.*) > 0
);

CREATE VIEW :"schema_brigades_name".ipv6_ula_nets_iweight AS (
    SELECT
        ipv6_ula_nets.id,
        ipv6_ula_nets.ipv6_net,
        COUNT(brigades.*) AS iweight 
    FROM
        :"schema_brigades_name".ipv6_ula_nets
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.ipv6_ula << ipv6_ula_nets.ipv6_net
    GROUP BY ipv6_ula_nets.ipv6_net
);

CREATE VIEW :"schema_brigades_name".ipv6_keydesk_nets_iweight AS (
    SELECT
        ipv6_keydesk_nets.id,
        ipv6_keydesk_nets.ipv6_net,
        COUNT(brigades.*) AS iweight
    FROM
        :"schema_brigades_name".ipv6_keydesk_nets
        LEFT JOIN :"schema_brigades_name".brigades ON brigades.keydesk_ipv6 << ipv6_keydesk_nets.ipv6_net
    GROUP BY ipv6_keydesk_nets.ipv6_net
);

CREATE TABLE :"schema_pairs_name".pairs_queue (
    queue_id serial PRIMARY KEY,
    payload json NOT NULL,
    error json
);

CREATE TABLE :"schema_brigades_name".brigades_queue (
    queue_id serial PRIMARY KEY,
    payload json NOT NULL,
    error json
);

CREATE ROLE :"pairs_dbuser" WITH LOGIN;
GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"pairs_dbuser";
GRANT SELECT,INSERT,UPDATE,DELETE ON ALL TABLES IN SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_brigades_name" TO :"pairs_dbuser";

CREATE ROLE :"brigades_dbuser" WITH LOGIN;
GRANT USAGE ON SCHEMA :"schema_brigades_name" TO :"brigades_dbuser";
GRANT SELECT ON :"schema_brigades_name".ipv4_cgnat_nets, :"schema_brigades_name".ipv6_ula_nets, :"schema_brigades_name".ipv6_keydesk_nets TO :"brigades_dbuser";
GRANT SELECT,UPDATE ON :"schema_brigades_name".active_pairs, :"schema_brigades_name".slots TO :"brigades_dbuser";
GRANT SELECT,UPDATE,INSERT,DELETE ON :"schema_brigades_name".brigades TO :"brigades_dbuser";
GRANT USAGE,SELECT,UPDATE ON ALL SEQUENCES IN SCHEMA :"schema_brigades_name"  TO :"brigades_dbuser";

CREATE SCHEMA :"schema_stats_name";
CREATE ROLE :"stats_dbuser" WITH LOGIN;
GRANT ALL PRIVILEGES ON SCHEMA :"schema_stats_name" TO :"stats_dbuser";
GRANT USAGE ON SCHEMA :"schema_pairs_name" TO :"stats_dbuser";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_pairs_name" TO :"stats_dbuser";
GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA :"schema_brigades_name"  TO :"stats_dbuser";

CREATE ROLE :"ministry_stats_dbuser" WITH LOGIN;
GRANT USAGE ON SCHEMA :"schema_stats_name" TO :"ministry_stats_dbuser";
GRANT SELECT ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"ministry_stats_dbuser";

GRANT USAGE ON SCHEMA :"schema_stats_name" TO :"brigades_dbuser";
GRANT SELECT,INSERT,DELETE ON ALL TABLES IN SCHEMA :"schema_stats_name" TO :"brigades_dbuser";

CREATE TABLE :"schema_stats_name".brigades_stats (
    brigade_id          uuid NOT NULL,
    create_at           timestamp without time zone NOT NULL DEFAULT NOW(),
    last_visit          timestamp without time zone DEFAULT NULL,
    user_count          int NOT NULL DEFAULT 0,
    FOREIGN KEY (brigade_id) REFERENCES :"schema_brigades_name".brigades (brigade_id) ON DELETE CASCADE
);

COMMIT;
