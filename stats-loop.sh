#!/bin/sh

DBNAME=${DBNAME:-"vgrealm"}
USERNAME="_marina_"

update() {
	id=$(echo "$1=========" | base32 -d 2>/dev/null | hexdump -ve '1/1 "%02x"')
	c="psql -d ${DBNAME} -q -v ON_ERROR_STOP=yes -t -A --set brigade_id=${id} --set create_at=${2} --set last_visit=${3} --set user_count=${4}"
	echo "${c}"
	${c} <<EOF
	BEGIN;
		INSERT INTO stats.brigades_stats (brigade_id, create_at) VALUES (:'brigade_id',to_timestamp(:create_at)) ON CONFLICT DO NOTHING;
		UPDATE 
			stats.brigades_stats 
		SET 
			last_visit=CASE WHEN :last_visit = -62135596800 THEN NULL ELSE to_timestamp(:last_visit) END,
			user_count=:user_count 
		WHERE 
			brigades_stats.brigade_id=:'brigade_id';
	COMMIT;
EOF
}

list=$(psql -d ${DBNAME} -v ON_ERROR_STOP=yes -t -A -c 'SELECT pair_id FROM pairs.pairs WHERE is_active=true ORDER BY control_ip')

for sid in ${list} ; do
        echo "[i] Server: ${sid}"

	ip=$(psql -d ${DBNAME} -q -v ON_ERROR_STOP=yes -t -A --set sid=${sid} <<EOF
SELECT control_ip FROM pairs.pairs WHERE pair_id=:'sid'
EOF
)

	brigades=$(psql -d ${DBNAME} -q -v ON_ERROR_STOP=yes -t -A -F " " --set sid=${sid} <<EOF
SELECT brigade_id FROM brigades.brigades WHERE pair_id=:'sid'
EOF
)
	list=""
	for i in ${brigades}; do
		list="${list},"$(echo "${i}" | xxd -r -p -l 16 | base32 | tr -d "=")
	done
	list=$(echo "${list}" | sed "s/^.\{1\}//")

	cmd="stats -b ${list}"
	echo "CMD: ${cmd}"
	output=$(ssh -o IdentitiesOnly=yes -o IdentityFile=~/.ssh/id_ecdsa -o StrictHostKeyChecking=no ${USERNAME}@${ip} ${cmd})
	rc=$?
	if [ $rc -ne 0 ]; then 
		echo "[-]         Something wrong: $rc"
		continue
	fi

	output=$(echo "${output}" | tail -n +2)
	
	count=$(echo "${output}" | jq -r '.stats|length')

	a=0
	while [ $a -lt ${count} ]; do
		args=$(echo "${output}" | jq -r ".stats[${a}]" | jq -r '. | "\(.brigade_id) \(.ctreate_at) \(.last_visit) \(.users_count)"')
		update ${args}
		a=$(expr $a + 1)
	done

	#args=$(echo "${output}" jq -r '.stats[] | "\(.brigade_id),\(.ctreate_at),\(.last_visit),\(.users_count)"')

        #exit
done