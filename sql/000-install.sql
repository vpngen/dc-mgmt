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

CREATE SCHEMA :schema_name;

-- External assignet nets.
CREATE TABLE :schema_name.ipv4_nets (
    ipv4_net cidr_ipv4 PRIMARY KEY NOT NULL,
    gateway  inet_ipv4_endpoint
);

-- CGNAT nets for clients.
CREATE TABLE :schema_name.ipv4_cgnat_nets (
    ipv4_net cidr_cgnat PRIMARY KEY NOT NULL
);

-- ULA nets for clients.
CREATE TABLE :schema_name.ipv6_ula_nets (
    ipv4_net cidr_ula PRIMARY KEY NOT NULL
);

-- Internal nets for infra. Control points range.
CREATE TABLE :schema_name.private_cidr_nets (
    ipv4_net cidr_private PRIMARY KEY NOT NULL
);

CREATE TABLE :schema_name.pairs (
    pair_id             uuid PRIMARY KEY NOT NULL,
    control_ip          inet UNIQUE NOT NULL,
    is_active           bool NOT NULL
);

CREATE TABLE :schema_name.pairs_endpoints_ipv4 (
    pair_id            uuid NOT NULL,
    endpoint_ipv4      inet_ipv4_endpoint UNIQUE NOT NULL,
    FOREIGN KEY (pair_id) REFERENCES :schema_name.pairs (pair_id)
);

CREATE TABLE :schema_name.brigades (
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
    FOREIGN KEY (pair_id) REFERENCES :schema_name.pairs (pair_id),
    FOREIGN KEY (endpoint_ipv4) REFERENCES :schema_name.pairs_endpoints_ipv4 (endpoint_ipv4)
);

CREATE VIEW :schema_name.active_pairs AS 
    SELECT 
        pairs.pair_id, 
        pairs.control_ip, 
        COUNT(pairs_endpoints_ipv4.*)-COUNT(brigades.*) AS free_slots 
    FROM 
        :schema_name.pairs, 
        :schema_name.pairs_endpoints_ipv4, 
        :schema_name.brigades
    WHERE
            pairs_endpoints_ipv4.pair_id=pairs.pair_id
        AND
            brigades.pair_id=pairs.pair_id
    GROUP BY pairs.pair_id
    HAVING
        COUNT(pairs_endpoints_ipv4.*)-COUNT(brigades.*) > 0
;

COMMIT;
