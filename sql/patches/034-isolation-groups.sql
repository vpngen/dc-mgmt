BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '034-isolation-groups', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role', '022-fix-brigades', '023-fix-brigades2', '024-fix-stats', '025-orders', '026-vgs-roles', '027-zones', '028-orders-ref', '029-nameservers', '030-viewsfixes', '031-socket-gen', '032-socket-gen-fix', '033-socket-gen-fix']);

CREATE TABLE IF NOT EXISTS pairs.isolation_groups (
        igrp_id uuid NOT NULL PRIMARY KEY DEFAULT gen_random_uuid(),
        description text NOT NULL DEFAULT '',
        update_time timestamp without time zone NOT NULL
);

ALTER TABLE pairs.pairs ADD COLUMN IF NOT EXISTS igrp_id uuid DEFAULT NULL REFERENCES pairs.isolation_groups(igrp_id);

COMMIT;