BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '025-orders', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats']);


CREATE TABLE IF NOT EXISTS pairs.endpoint_nums (
        endpoint_num integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
        update_time timestamp without time zone NOT NULL
);

CREATE TABLE IF NOT EXISTS pairs.pair_orders (
        order_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
        endpoint_num integer NOT NULL REFERENCES pairs.endpoint_nums(endpoint_num),
        brigade_id uuid NOT NULL,
        brigade_name text NOT NULL,
        action text NOT NULL,
        created_at timestamp without time zone NOT NULL,
        is_processing boolean NOT NULL DEFAULT false,
        processing_started_at timestamp without time zone DEFAULT NULL,
        is_registering boolean NOT NULL DEFAULT false,
        registering_started_at timestamp without time zone DEFAULT NULL,
        is_filling boolean NOT NULL DEFAULT false,
        filling_started_at timestamp without time zone DEFAULT NULL,
        is_completed boolean NOT NULL DEFAULT false,
        completed_at timestamp without time zone DEFAULT NULL,
        is_error boolean NOT NULL DEFAULT false,
        error_at timestamp without time zone DEFAULT NULL,
        message text NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX pair_orders_order_id_idx ON pairs.pair_orders (order_id);

CREATE TABLE IF NOT EXISTS pairs.endpoint_num_links (
        endpoint_num integer NOT NULL REFERENCES pairs.endpoint_nums(endpoint_num),
        endpoint_ipv4 inet_ipv4_endpoint NOT NULL REFERENCES pairs.pairs_endpoints_ipv4(endpoint_ipv4)
);

CREATE UNIQUE INDEX endpoint_num_links_endpoint_num_idx ON pairs.endpoint_num_links (endpoint_num);
CREATE UNIQUE INDEX endpoint_num_links_endpoint_ipv4_idx ON pairs.endpoint_num_links (endpoint_ipv4);

COMMIT;