BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '025-orders', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats']);


CREATE TABLE IF NOT EXISTS pairs.pair_nums (
        pair_num integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
        update_time timestamp without time zone NOT NULL
);

CREATE TABLE IF NOT EXISTS pairs.pair_orders (
        order_id uuid PRIMARY KEY,
        pair_num integer NOT NULL REFERENCES pairs.pair_nums(pair_num),
        brigade_id uuid NOT NULL,
        brigade_name text NOT NULL,
        created_at timestamp without time zone NOT NULL,
        is_processing boolean NOT NULL DEFAULT false,
        processing_started_at timestamp without time zone,
        is_registering boolean NOT NULL DEFAULT false,
        registering_started_at boolean NOT NULL DEFAULT false,
        is_filling boolean NOT NULL DEFAULT false,
        filling_started_at boolean NOT NULL DEFAULT false,
        is_completed boolean NOT NULL DEFAULT false,
        completed_at timestamp without time zone,
        is_error boolean NOT NULL DEFAULT false,
        error_at timestamp without time zone,
        message text NOT NULL DEFAULT ''
);

CREATE UNIQUE INDEX pair_orders_order_id_idx ON pairs.pair_orders (order_id);

CREATE TABLE IF NOT EXISTS pairs.pair_num_links (
        pair_num integer NOT NULL REFERENCES pairs.pair_nums(pair_num),
        pair_id uuid NOT NULL REFERENCES pairs.pairs(pair_id)
);

CREATE UNIQUE INDEX pair_num_links_pair_num ON pairs.pair_num_links (pair_num);
CREATE UNIQUE INDEX pair_num_links_pair_id ON pairs.pair_num_links (pair_id);

COMMIT;