BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '022-fix-brigades', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps', '012-brigades-instance', '013-slot-flags', '014-viewfixes', '015-stats', '016-viewfixes', '017-viewfixes', '018-viewfixes', '019-ctrl-keys', '020-viewfixes', '021-vgmigr-role']);

ALTER TABLE :"schema_brigades_name".brigades DROP CONSTRAINT brigades_brigadier_key; 

ALTER TABLE :"schema_brigades_name".brigades ADD CONSTRAINT unique_brigadier_per_id UNIQUE (id, brigadier);

COMMIT;