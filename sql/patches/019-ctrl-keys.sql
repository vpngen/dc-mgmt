BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '019-ctrl-keys', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes']);

ALTER TABLE :"schema_pairs_name".pairs ADD COLUMN router_nacl_pubkey text NOT NULL DEFAULT '';
ALTER TABLE :"schema_pairs_name".pairs ADD COLUMN ssh_ed25519_pubkey text NOT NULL DEFAULT '';

COMMIT;