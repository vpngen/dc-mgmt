BEGIN;

SELECT _v.assert_user_is_superuser();

SELECT _v.register_patch( '012-brigades-instance', ARRAY[ '001-init', '002-roles', '003-stats', '004-stats', '005-stats', '006-stats', '007-stats', '008-domains', '009-domains', '010-viewfixes', '011-collectsnaps']);

DROP table IF EXISTS :"schema_brigades_name".brigades_statistics;

ALTER TABLE :"schema_brigades_name".brigades ADD COLUMN instance_id uuid NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE :"schema_brigades_name".brigades DROP CONSTRAINT brigades_pkey CASCADE;
ALTER TABLE :"schema_brigades_name".brigades ADD PRIMARY KEY (brigade_id, instance_id);
ALTER TABLE :"schema_brigades_name".brigades ALTER COLUMN instance_id DROP DEFAULT;

ALTER TABLE :"schema_brigades_name".brigades ADD COLUMN main bool NOT NULL DEFAULT true;
ALTER TABLE :"schema_brigades_name".brigades ALTER COLUMN main DROP DEFAULT;

CREATE UNIQUE INDEX brigades_brigade_id_main_true_idx ON :"schema_brigades_name".brigades (brigade_id) WHERE main=TRUE;

ALTER TABLE :"schema_brigades_name".brigades_stats ADD COLUMN instance_id uuid DEFAULT NULL;
UPDATE :"schema_brigades_name".brigades_stats SET instance_id = b.instance_id FROM :"schema_brigades_name".brigades b WHERE brigades_stats.brigade_id = b.brigade_id;
DELETE FROM :"schema_brigades_name".brigades_stats WHERE instance_id IS NULL;

ALTER TABLE :"schema_brigades_name".brigades_stats ALTER COLUMN instance_id SET NOT NULL;
ALTER TABLE :"schema_brigades_name".brigades_stats ALTER COLUMN instance_id DROP DEFAULT;
ALTER TABLE :"schema_brigades_name".brigades_stats ADD CONSTRAINT fk_brigades_instance_id FOREIGN KEY (instance_id) REFERENCES "schema_brigades_name".brigades(instance_id) ON DELETE CASCADE ON UPDATE CASCADE;
ALTER TABLE :"schema_brigades_name".brigades_stats ADD PRIMARY KEY (brigade_id, instance_id);


COMMIT;