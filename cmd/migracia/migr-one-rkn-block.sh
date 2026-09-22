#!/bin/sh
# migr-one-rkn-block.sh — move one brigade off an RKN-blocked endpoint address.
#
# Driven by a support ticket that names a brigade by domain or by its current
# endpoint address. Resolves the brigade, confirms with the external monitor
# that the address really is blocked from Russia, picks a target pair, runs
# 01-migr-one-propagade-x.sh, verifies the move in the database, releases the
# reservation, and re-checks the new address. If the new address is blocked
# too it tries again on the next pair.
#
# Run as vgmigr on the head node, inside screen or tmux.
#
#   ./migr-one-rkn-block.sh api.jaggyslawa.org
#   ./migr-one-rkn-block.sh 141.98.4.154 -n
#   ./migr-one-rkn-block.sh crm.example.org -vip -attempts 3
#
# WHAT IT DELIBERATELY DOES NOT DO
#
# It never calls 02-migr-one-cleanup.sh. The instance left behind on the old
# address keeps answering on 80 and 443, which is what lets the monitor test
# that address every day until the ban lifts. Destroying it would remove the
# only probe we have. Reclaiming the slot later is a separate job: destroy the
# probe by its base32 id on its own control node, delete the instance row, and
# the address returns to brigades.slots on its own.
#
# It also never disables the old endpoint. The parked instance already keeps
# the address out of brigades.slots, so there is nothing to disable and
# nothing to re-enable afterwards.

set -e

SELFDIR="$(cd "$(dirname "$0")" && pwd)"

MONITOR_URL=${MONITOR_URL:-"https://monitor.baza-info.org/api/v1/check"}
MONITOR_TIMEOUT=${MONITOR_TIMEOUT:-"30"}
MONITOR_TRIES=${MONITOR_TRIES:-"3"}
DBNAME=${DBNAME:-"vgrealm"}
PROPAGADE=${PROPAGADE:-"${SELFDIR}/01-migr-one-propagade-x.sh"}
RESERVATION_TOOL=${RESERVATION_TOOL:-"/opt/vg-dc-snaps/create_reservation.sh"}
MNT_MIN=${MNT_MIN:-"15"}
# How many blocked addresses a /24 needs before the whole range is skipped.
BADNET_MIN_BLOCKED=${BADNET_MIN_BLOCKED:-"10"}
LOGDIR=${LOGDIR:-"${HOME}/migr-logs"}
JOURNAL="${LOGDIR}/rkn-block-journal.tsv"

ATTEMPTS="2"
DRYRUN=""
VIP=""
PACK=""
FORCE=""
ASSUME_YES=""
CTRL_OVERRIDE=""
TARGET=""

usage () {
	cat <<'EOF'
Usage: migr-one-rkn-block.sh <domain|ip> [options]

  -n, --dry-run     resolve, check and choose, then print what would run
  -attempts N       how many migrations to try if the new address is also
                    blocked (default 2). 0 means check and report only.
  -mnt MIN          keydesk maintenance window in minutes (default 15)
  -ctrl CIDR        force the first target pair, e.g. 10.30.38.10/32
  -vip              prefer pairs inside VIP_INTERNAL_NETWORKS
  -pack             fill the fullest healthy pair instead of the emptiest
  -force            proceed even if the monitor says the current address
                    is not blocked, or stale working files exist
  -y, --yes         do not ask for confirmation
EOF
}

die  () { echo "[-] $*" >&2; exit 1; }
info () { echo "[i] $*"; }
warn () { echo "[!] $*" >&2; }
rule () { echo "------------------------------------------------------------"; }

while [ $# -gt 0 ]; do
	case "$1" in
		-h|--help)     usage; exit 0 ;;
		-n|--dry-run)  DRYRUN="x";        shift ;;
		-y|--yes)      ASSUME_YES="x";    shift ;;
		-vip)          VIP="x";           shift ;;
		-pack)         PACK="x";          shift ;;
		-force)        FORCE="x";         shift ;;
		-attempts)     ATTEMPTS="$2";     shift 2 ;;
		-mnt)          MNT_MIN="$2";      shift 2 ;;
		-ctrl)         CTRL_OVERRIDE="$2"; shift 2 ;;
		-*)            die "unknown option: $1" ;;
		*)
			if [ -n "${TARGET}" ]; then
				die "more than one target given: ${TARGET} and $1"
			fi
			TARGET="$1"
			shift
			;;
	esac
done

[ -n "${TARGET}" ] || { usage; exit 1; }

echo "${TARGET}" | grep -Eq '^[A-Za-z0-9.-]+$' \
	|| die "target must be a domain or an IPv4 address: ${TARGET}"

case "${ATTEMPTS}" in ''|*[!0-9]*) die "-attempts must be a number" ;; esac
case "${MNT_MIN}"  in ''|*[!0-9]*) die "-mnt must be a number" ;; esac

if [ -n "${CTRL_OVERRIDE}" ]; then
	echo "${CTRL_OVERRIDE}" | grep -Eq '^[0-9]+(\.[0-9]+){3}/[0-9]+$' \
		|| die "-ctrl must be a CIDR, e.g. 10.30.38.10/32"
fi

[ -x "${PROPAGADE}" ] || die "not executable: ${PROPAGADE}"
command -v jq   >/dev/null 2>&1 || die "jq not found"
command -v curl >/dev/null 2>&1 || die "curl not found"

mkdir -p "${LOGDIR}"

if [ -z "${STY}${TMUX}" ] && [ -z "${DRYRUN}" ]; then
	warn "not inside screen or tmux; a dropped session during the switch is hard to recover"
fi

# ---------------------------------------------------------------- helpers

psql_q () {
	psql -d "${DBNAME}" -q -t -A -F'|' -v ON_ERROR_STOP=1 -c "$1"
}

# Same query, laid out for a human rather than for cut(1).
psql_table () {
	if command -v column >/dev/null 2>&1; then
		psql_q "$1" | column -t -s'|' | sed 's/^/    /'
	else
		psql_q "$1" | sed 's/^/    /'
	fi
}

# Ask the monitor about one target. Prints the JSON body on success.
# The service can take up to about 15 seconds to answer.
monitor_raw () {
	_target="$1"
	_try="0"

	while [ "${_try}" -lt "${MONITOR_TRIES}" ]; do
		_try=$((_try + 1))

		_body="$(curl -sS --max-time "${MONITOR_TIMEOUT}" \
			-X POST \
			-H 'Content-Type: application/json' \
			--data "{\"target\":\"${_target}\"}" \
			"${MONITOR_URL}" 2>/dev/null || true)"

		if [ -n "${_body}" ] && echo "${_body}" | jq -e '.verdict' >/dev/null 2>&1; then
			echo "${_body}"

			return 0
		fi

		warn "monitor attempt ${_try}/${MONITOR_TRIES} for ${_target} gave no verdict"
		sleep 5
	done

	return 1
}

monitor_report () {
	echo "$1" \
		| jq -r '[.verdict, (.summary // "-"), (.detail // "-"),
		          (.reachable_from // 0), (.checked_from // 0)] | @tsv' \
		| while IFS="$(printf '\t')" read -r _v _s _d _r _c; do
			echo "    verdict:  ${_v}"
			echo "    summary:  ${_s}"
			echo "    detail:   ${_d}"
			echo "    checked:  ${_r}/${_c} reachable from Russia"
		done
}

# VIP networks come from the same variable addbrigade and get_free_slots read,
# so there is one source of truth for which pairs are VIP.
build_vip_expr () {
	_e=""

	for _n in $(echo "${VIP_INTERNAL_NETWORKS}" | tr ',' ' '); do
		if ! echo "${_n}" | grep -Eq '^[0-9]+(\.[0-9]+){3}/[0-9]+$'; then
			die "bad entry in VIP_INTERNAL_NETWORKS: ${_n}"
		fi

		if [ -n "${_e}" ]; then
			_e="${_e} OR "
		fi

		_e="${_e}s.control_ip <<= '${_n}'::inet"
	done

	if [ -z "${_e}" ]; then
		echo "false"
	else
		echo "(${_e})"
	fi
}

# Compaction: the fullest pair that still has a free slot wins, so pairs fill
# up instead of the fleet spreading thin. create_reservation.sh orders slots by
# free_slots_count DESC, i.e. the opposite, so we hand it a single control
# address as a /32 and there is nothing left for it to choose between.
pick_pair () {
	_exclude="$1"
	_badnets="$2"
	_notin=""
	_nonet=""

	if [ -n "${_exclude}" ]; then
		_notin="AND s.control_ip NOT IN (${_exclude})"
	fi

	# A blocked address is rarely blocked alone. Endpoints are handed out
	# contiguously per pair, so once one address in a /24 comes back
	# blocked_ru, skip every pair whose endpoints live in that range.
	if [ -n "${_badnets}" ]; then
		_nonet="AND NOT (${_badnets})"
	fi

	if [ -n "${VIP}" ]; then
		_order="CASE WHEN c.is_vip THEN 0 ELSE 1 END"
	else
		_order="CASE WHEN c.is_vip THEN 1 ELSE 0 END"
	fi

	# A pair is full because its node carries the most brigades. Packing by
	# default walked this script onto the least healthy node in the fleet
	# twice on 2026-09-21: 220 MB disk, /run too small to reload systemd,
	# load average 18. So spread by default and pack only on request.
	if [ -n "${PACK}" ]; then
		_load="c.free_slots ASC"
	else
		_load="c.free_slots DESC"
	fi

	if [ -z "${REACH}" ]; then
		psql_q "SELECT host(c.control_ip), c.free_slots, -1, -1
FROM (
	SELECT s.control_ip,
	       count(*) AS free_slots,
	       bool_or(${VIP_EXPR}) AS is_vip
	FROM brigades.slots s
	JOIN brigades.active_pairs p ON s.pair_id = p.pair_id
	WHERE s.domain_name IS NULL ${_notin} ${_nonet}
	GROUP BY s.control_ip
) c
ORDER BY ${_order} ASC, ${_load}, c.control_ip ASC
LIMIT 1;"

		return
	fi

	# Three signals, in order of how much they cost us to get wrong:
	#   blocked in the /24  - lands the brigade straight back behind the ban
	#   unhealthy addresses - the node cannot serve what it already has
	#   load                - only breaks ties between equally good pairs
	# no_listener is NOT unhealthy: a free slot has nothing listening by
	# definition, and counting it would penalise every pair with capacity.
	psql_q "WITH latest AS (
	SELECT DISTINCT ON (endpoint_ipv4) endpoint_ipv4, verdict
	FROM stats.endpoint_reachability
	ORDER BY endpoint_ipv4, observed_at DESC
), netblocked AS (
	SELECT network(set_masklen(endpoint_ipv4,24)) AS net,
	       count(*) FILTER (WHERE verdict = 'blocked_ru') AS blocked
	FROM latest
	GROUP BY 1
), pairhealth AS (
	SELECT p.control_ip,
	       count(*) FILTER (WHERE l.verdict IN ('partial','blocked_ru')) AS bad
	FROM pairs.pairs p
	JOIN pairs.pairs_endpoints_ipv4 pe ON pe.pair_id = p.pair_id
	LEFT JOIN latest l ON l.endpoint_ipv4 = pe.endpoint_ipv4
	GROUP BY 1
), cand AS (
	SELECT s.control_ip,
	       network(set_masklen(s.endpoint_ipv4,24)) AS net,
	       count(*) AS free_slots,
	       bool_or(${VIP_EXPR}) AS is_vip
	FROM brigades.slots s
	JOIN brigades.active_pairs p ON s.pair_id = p.pair_id
	WHERE s.domain_name IS NULL ${_notin} ${_nonet}
	GROUP BY 1, 2
)
SELECT host(c.control_ip), c.free_slots,
       coalesce(n.blocked, 0), coalesce(h.bad, 0)
FROM cand c
LEFT JOIN netblocked n ON c.net = n.net
LEFT JOIN pairhealth h ON h.control_ip = c.control_ip
ORDER BY ${_order} ASC, coalesce(n.blocked,0) ASC, coalesce(h.bad,0) ASC,
         ${_load}, c.control_ip ASC
LIMIT 1;"
}

journal () {
	printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
		"$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" "$2" "$3" "$4" "$5" "$6" "$7" \
		>> "${JOURNAL}"
}

# Confirmed VIP control networks. addbrigade reads the same variable name, so
# set it in the environment where that is configured and this follows.
VIP_INTERNAL_NETWORKS=${VIP_INTERNAL_NETWORKS:-"10.30.33.0/24,10.30.53.0/24"}

VIP_EXPR="$(build_vip_expr)"

# stats.endpoint_reachability is filled by reachability-ingest. If it is
# missing or stale this degrades to density-only selection rather than failing.
REACH=""
_reach_rows="$(psql -d "${DBNAME}" -q -t -A -v ON_ERROR_STOP=1 -c \
	"SELECT count(*) FROM stats.endpoint_reachability
	 WHERE observed_at > now() - interval '7 days';" 2>/dev/null || true)"

case "${_reach_rows}" in
	''|*[!0-9]*) _reach_rows="0" ;;
esac

if [ "${_reach_rows}" -gt 0 ]; then
	REACH="x"
else
	warn "no recent rows in stats.endpoint_reachability:"
	warn "ranking target pairs by density alone, which cannot avoid blocked ranges"
fi

# -vip is only meaningful if we know which networks are VIP. The variable is
# the same one addbrigade and get_free_slots read, so set it in the
# environment rather than hard-coding addresses here. Saying nothing when the
# list is empty would quietly send a VIP brigade to an ordinary pair.
if [ -n "${VIP}" ] && [ "${VIP_EXPR}" = "false" ]; then
	warn "-vip given but VIP_INTERNAL_NETWORKS is empty, so there are no"
	warn "VIP pairs to prefer. Falling back to ordinary selection."
	warn "Set it to use the flag, for example:"
	warn "    VIP_INTERNAL_NETWORKS=10.30.33.0/24,10.30.53.0/24 $0 ${TARGET} -vip"
fi

# ---------------------------------------------------------------- resolve

if echo "${TARGET}" | grep -Eq '^[0-9]+(\.[0-9]+){3}$'; then
	WHERE="b.endpoint_ipv4 = '${TARGET}'::inet"
	BY="address"
else
	WHERE="b.domain_name = '${TARGET}'"
	BY="domain"
fi

ROW="$(psql_q "SELECT b.brigade_id,
       host(b.endpoint_ipv4),
       coalesce(b.domain_name, ''),
       b.instance_id,
       host(p.control_ip),
       b.main
FROM brigades.brigades b
JOIN pairs.pairs p ON p.pair_id = b.pair_id
WHERE ${WHERE};")"

[ -n "${ROW}" ] || die "no brigade found by ${BY}: ${TARGET}"

if [ "$(echo "${ROW}" | wc -l)" -ne 1 ]; then
	echo "${ROW}"
	die "ambiguous: ${TARGET} matches more than one row"
fi

BID="$(echo    "${ROW}" | cut -d'|' -f1)"
B_IP="$(echo   "${ROW}" | cut -d'|' -f2)"
B_DOM="$(echo  "${ROW}" | cut -d'|' -f3)"
B_INST="$(echo "${ROW}" | cut -d'|' -f4)"
B_CTRL="$(echo "${ROW}" | cut -d'|' -f5)"
B_MAIN="$(echo "${ROW}" | cut -d'|' -f6)"

# A non-main row is a parked probe from an earlier migration. Migrating it
# would move a copy nobody uses and leave the live brigade where it is.
if [ "${B_MAIN}" != "t" ]; then
	MAINROW="$(psql_q "SELECT host(endpoint_ipv4), coalesce(domain_name, '')
FROM brigades.brigades WHERE brigade_id = '${BID}' AND main;")"

	echo >&2
	warn "${TARGET} is NOT the main instance of this brigade."
	warn "It is a parked leftover on ${B_IP} (instance ${B_INST})."

	if [ -n "${MAINROW}" ]; then
		warn "The main instance is:"
		warn "    address: $(echo "${MAINROW}" | cut -d'|' -f1)"
		warn "    domain:  $(echo "${MAINROW}" | cut -d'|' -f2)"
		warn "Rerun with:  $0 $(echo "${MAINROW}" | cut -d'|' -f1)"
	else
		warn "This brigade has NO main instance. Do not migrate; investigate first."
	fi

	exit 1
fi

rule
info "brigade   ${BID}"
info "domain    ${B_DOM}"
info "address   ${B_IP}"
info "control   ${B_CTRL}"
rule
info "all instances of this brigade:"
psql_table "SELECT host(b.endpoint_ipv4), coalesce(b.domain_name,'(parked)'),
       host(p.control_ip), b.main, b.instance_id
FROM brigades.brigades b
JOIN pairs.pairs p ON p.pair_id = b.pair_id
WHERE b.brigade_id = '${BID}'
ORDER BY b.main DESC, b.endpoint_ipv4;"
rule

# ------------------------------------------------- is it really blocked?

info "asking the monitor about the current address ${B_IP} ..."

if ! PRE="$(monitor_raw "${B_IP}")"; then
	if [ -z "${FORCE}" ]; then
		die "monitor gave no answer for ${B_IP}; rerun with -force to migrate anyway"
	fi

	warn "monitor gave no answer; -force given, continuing"
	PRE=""
else
	monitor_report "${PRE}"
	PRE_VERDICT="$(echo "${PRE}" | jq -r '.verdict')"

	if [ "${PRE_VERDICT}" != "blocked_ru" ]; then
		warn "verdict is '${PRE_VERDICT}', not 'blocked_ru'."
		warn "The address answers from Russia, so moving the brigade would not help"
		warn "and would cost a slot plus a keydesk outage."

		if [ -z "${FORCE}" ]; then
			die "refusing to migrate; rerun with -force if you are sure"
		fi

		warn "-force given, continuing anyway"
	fi
fi

rule

# ---------------------------------------------------------------- the loop

CUR_IP="${B_IP}"
ATTEMPT="0"

# Every instance of this brigade sits on an address it was moved off, which
# means every one of them was blocked. Seed the exclusions from all of them,
# not just the current address, so a rerun after an interrupted session does
# not walk straight back into a range we already know is bad.
EXCLUDE=""
BADNETS=""

for _pair in $(psql_q "SELECT host(control_ip) || ' ' || host(endpoint_ipv4)
FROM brigades.brigades b JOIN pairs.pairs p ON p.pair_id = b.pair_id
WHERE b.brigade_id = '${BID}';" | tr ' ' '|'); do
	_ctrl="$(echo "${_pair}" | cut -d'|' -f1)"
	_ep="$(echo   "${_pair}" | cut -d'|' -f2)"
	_net="$(echo  "${_ep}"   | cut -d. -f1-3).0/24"

	if [ -n "${EXCLUDE}" ]; then
		EXCLUDE="${EXCLUDE},"
	fi

	EXCLUDE="${EXCLUDE}'${_ctrl}'::inet"

	case "${BADNETS}" in
		*"'${_net}'"*) continue ;;
	esac

	# Excluding a whole /24 is right when the range is genuinely under
	# attack and wrong when one address in a healthy range was blocked.
	# RKN blocks individual addresses: measured 2026-09-21, 194.87.51.142
	# was dark while 9 of the 16 slots on its own node had Russian clients
	# within the hour. Only widen to the range when the range looks targeted.
	_blk="0"

	if [ -n "${REACH}" ]; then
		_blk="$(psql -d "${DBNAME}" -q -t -A -v ON_ERROR_STOP=1 -c \
			"WITH latest AS (
				SELECT DISTINCT ON (endpoint_ipv4) endpoint_ipv4, verdict
				FROM stats.endpoint_reachability
				ORDER BY endpoint_ipv4, observed_at DESC)
			 SELECT count(*) FILTER (WHERE verdict = 'blocked_ru')
			 FROM latest WHERE endpoint_ipv4 <<= '${_net}'::inet;" 2>/dev/null || true)"

		case "${_blk}" in
			''|*[!0-9]*) _blk="0" ;;
		esac
	fi

	if [ "${_blk}" -lt "${BADNET_MIN_BLOCKED}" ]; then
		info "    ${_net}: ${_blk} blocked address(es), keeping the range in play"
		continue
	fi

	if [ -n "${BADNETS}" ]; then
		BADNETS="${BADNETS} OR "
	fi

	BADNETS="${BADNETS}s.endpoint_ipv4 <<= '${_net}'::inet"
done

if [ -n "${BADNETS}" ]; then
	info "avoiding these endpoint ranges (targeted enough to skip entirely):"
	echo "${BADNETS}" | sed "s/ OR /\n/g; s/s.endpoint_ipv4 <<= //g; s/'//g; s/::inet//g" | sed 's/^/    /'
else
	info "no range is targeted enough to exclude; avoiding this brigade's own pairs only"
fi
rule
STATUS="unknown"
FINAL_IP="${B_IP}"

if [ "${ATTEMPTS}" -eq 0 ]; then
	info "-attempts 0: report only, nothing migrated"
	exit 0
fi

while [ "${ATTEMPT}" -lt "${ATTEMPTS}" ]; do
	ATTEMPT=$((ATTEMPT + 1))

	if [ -n "${CTRL_OVERRIDE}" ] && [ "${ATTEMPT}" -eq 1 ]; then
		PAIR_IP="${CTRL_OVERRIDE%%/*}"
		PAIR_FREE="forced"
		PAIR_BLK="-1"
		PAIR_BAD="-1"
	else
		PICK="$(pick_pair "${EXCLUDE}" "${BADNETS}")"

		if [ -z "${PICK}" ]; then
			warn "no pair left with a free slot"
			STATUS="no_slots"
			break
		fi

		PAIR_IP="$(echo   "${PICK}" | cut -d'|' -f1)"
		PAIR_FREE="$(echo "${PICK}" | cut -d'|' -f2)"
		PAIR_BLK="$(echo  "${PICK}" | cut -d'|' -f3)"
		PAIR_BAD="$(echo  "${PICK}" | cut -d'|' -f4)"
	fi

	info "attempt ${ATTEMPT}/${ATTEMPTS}"
	info "    from      ${CUR_IP}"
	if [ "${PAIR_BLK}" = "-1" ]; then
		info "    to pair   ${PAIR_IP}  (free slots: ${PAIR_FREE}, range history: unknown)"
	else
		info "    to pair   ${PAIR_IP}  (free slots: ${PAIR_FREE}, blocked in its /24: ${PAIR_BLK}, unhealthy addresses on the pair: ${PAIR_BAD})"
	fi

	# A tag of our own so the propagade script cannot pick up a snapshot or a
	# reservation left behind by an earlier run with the same address and pair.
	RUN_TAG="${CUR_IP}-${PAIR_IP}-0.0.0.0-$(date -u +%Y%m%d%H%M%S)"
	MNT_TO="$(date -d "+${MNT_MIN} min" +%s)"
	LOG="${LOGDIR}/rkn-${B_DOM:-${CUR_IP}}-$(date -u +%Y%m%d%H%M%S).log"

	# Belt and braces: with a timestamped tag these cannot exist, but a plain
	# tag left by a hand-run would be reused silently, so say so.
	STALE="$(ls -d "/vg-snapshots/${CUR_IP}-${PAIR_IP}-0.0.0.0-migr" 2>/dev/null || true)"
	STALE="${STALE} $(ls "${HOME}/tmp/${CUR_IP}-${PAIR_IP}-0.0.0.0-migr-reserv-"*.json 2>/dev/null || true)"

	if [ -n "$(echo "${STALE}" | tr -d ' ')" ]; then
		warn "working files from an earlier hand-run exist for this address and pair:"
		echo "${STALE}" | tr ' ' '\n' | grep -v '^$' | sed 's/^/    /' >&2
		warn "this run uses its own tag ${RUN_TAG} and will not touch them"
	fi

	info "    tag       ${RUN_TAG}"
	info "    log       ${LOG}"

	if [ -n "${DRYRUN}" ]; then
		rule
		info "dry run, would execute:"
		echo "    BASENET=\"${RUN_TAG}\" MNT_TO=\"${MNT_TO}\" \\"
		echo "        ${PROPAGADE} -ip ${CUR_IP} -ctrl ${PAIR_IP}/32"
		echo "    ${RESERVATION_TOOL} delete -f <reservation>"
		rule
		exit 0
	fi

	if [ -z "${ASSUME_YES}" ]; then
		printf 'Migrate %s from %s to pair %s? [y/N] ' "${B_DOM:-${BID}}" "${CUR_IP}" "${PAIR_IP}"
		read -r ANS || ANS=""

		case "${ANS}" in
			y|Y|yes|YES) : ;;
			*) die "aborted by operator" ;;
		esac
	fi

	RCFILE="$(mktemp)"
	{
		BASENET="${RUN_TAG}" MNT_TO="${MNT_TO}" \
			"${PROPAGADE}" -ip "${CUR_IP}" -ctrl "${PAIR_IP}/32" 2>&1
		echo "$?" > "${RCFILE}"
	} | tee -a "${LOG}"
	RC="$(cat "${RCFILE}")"
	rm -f "${RCFILE}"

	# The reservation is authoritative from its own file, never scraped from
	# the log, so a partial line cannot give us the wrong UUID.
	RESV=""
	RESV_FILE="$(ls "${HOME}/tmp/${RUN_TAG}-migr-reserv-"*.json 2>/dev/null | tail -n 1 || true)"

	if [ -n "${RESV_FILE}" ] && [ -s "${RESV_FILE}" ]; then
		RESV="$(jq -r '.reservation_id' < "${RESV_FILE}")"
	fi

	if [ "${RC}" != "0" ] || grep -q '^!!!' "${LOG}"; then
		rule
		warn "the propagade script reported a failure (exit ${RC})"
		grep '^!!!' "${LOG}" | sed 's/^/    /' >&2 || true
		warn "NOT releasing the reservation: a half-finished migration needs a look first"

		if [ -n "${RESV}" ]; then
			warn "when you have checked it, release the slot with:"
			warn "    ${RESERVATION_TOOL} delete -f ${RESV}"
		fi

		journal "${BID}" "${B_DOM}" "${CUR_IP}" "-" "${PAIR_IP}" "${RESV:--}" "propagade_failed"
		die "stopping after a failed migration"
	fi

	# The database is the authority on whether the brigade actually moved.
	# switch_local_migr.sh can exit 0 and still skip a brigade.
	NEW_IP="$(psql_q "SELECT host(endpoint_ipv4) FROM brigades.brigades
WHERE brigade_id = '${BID}' AND main;")"
	PARKED="$(psql_q "SELECT count(*) FROM brigades.brigades
WHERE brigade_id = '${BID}' AND NOT main AND endpoint_ipv4 = '${CUR_IP}'::inet;")"

	if [ -z "${NEW_IP}" ] || [ "${NEW_IP}" = "${CUR_IP}" ] || [ "${PARKED}" = "0" ]; then
		rule
		warn "the database does not show a completed move:"
		warn "    main address now: ${NEW_IP:-none}"
		warn "    parked row on ${CUR_IP}: ${PARKED}"
		warn "NOT releasing the reservation."

		if [ -n "${RESV}" ]; then
			warn "    ${RESERVATION_TOOL} delete -f ${RESV}"
		fi

		journal "${BID}" "${B_DOM}" "${CUR_IP}" "-" "${PAIR_IP}" "${RESV:--}" "switch_skipped"
		die "stopping: the switch reported success but the brigade did not move"
	fi

	info "moved: ${CUR_IP} -> ${NEW_IP}, old instance parked as the probe"
	FINAL_IP="${NEW_IP}"

	# Release the booking. This removes rows in reserved_endpoints_ipv4 and the
	# reservations row, nothing else; it cannot touch a brigade. The -f matters:
	# without it the parent delete hits the foreign key and the whole
	# transaction rolls back, because the address is now brigade-held.
	if [ -n "${RESV}" ]; then
		info "releasing reservation ${RESV}"

		if "${RESERVATION_TOOL}" delete -f "${RESV}" >> "${LOG}" 2>&1; then
			info "reservation released"
		else
			warn "reservation release FAILED, do this by hand:"
			warn "    ${RESERVATION_TOOL} delete -f ${RESV}"
		fi
	else
		warn "no reservation file found for ${RUN_TAG}; nothing released"
	fi

	# Did we land somewhere usable?
	info "asking the monitor about the new address ${NEW_IP} ..."

	if ! POST="$(monitor_raw "${NEW_IP}")"; then
		warn "monitor gave no answer for ${NEW_IP}; check it by hand"
		STATUS="unchecked"
		journal "${BID}" "${B_DOM}" "${CUR_IP}" "${NEW_IP}" "${PAIR_IP}" "${RESV:--}" "unchecked"
		break
	fi

	monitor_report "${POST}"
	VERDICT="$(echo "${POST}" | jq -r '.verdict')"
	journal "${BID}" "${B_DOM}" "${CUR_IP}" "${NEW_IP}" "${PAIR_IP}" "${RESV:--}" "${VERDICT}"

	if [ "${VERDICT}" = "blocked_ru" ]; then
		warn "the new address is blocked too"
		STATUS="blocked_ru"
		EXCLUDE="${EXCLUDE},'${PAIR_IP}'::inet"
		BADNETS="${BADNETS} OR s.endpoint_ipv4 <<= '$(echo "${NEW_IP}" | cut -d. -f1-3).0/24'::inet"
		CUR_IP="${NEW_IP}"
		rule
		continue
	fi

	STATUS="${VERDICT}"
	break
done

# ---------------------------------------------------------------- report

rule
info "brigade   ${BID}"
info "domain    ${B_DOM}"
info "started   ${B_IP}"
info "now on    ${FINAL_IP}"
info "attempts  ${ATTEMPT}"
info "verdict   ${STATUS}"
rule
info "instances now (every parked one is a probe the monitor tests daily):"
psql_table "SELECT host(b.endpoint_ipv4), coalesce(b.domain_name,'(parked)'),
       host(p.control_ip), b.main
FROM brigades.brigades b
JOIN pairs.pairs p ON p.pair_id = b.pair_id
WHERE b.brigade_id = '${BID}'
ORDER BY b.main DESC, b.endpoint_ipv4;"
rule
info "journal   ${JOURNAL}"

case "${STATUS}" in
	blocked_ru) warn "still on a blocked address after ${ATTEMPT} attempt(s)"; exit 4 ;;
	no_slots)   warn "ran out of candidate pairs"; exit 5 ;;
	unchecked)  exit 6 ;;
	*)          info "done"; exit 0 ;;
esac
