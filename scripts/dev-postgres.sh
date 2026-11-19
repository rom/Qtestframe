#!/usr/bin/env bash
# Throw-away PostgreSQL 16 for local development (Linux and macOS), using
# podman or docker. Listens on 127.0.0.1 only.
#
#   scripts/dev-postgres.sh start      # start (idempotent) and print the DSN
#   eval "$(scripts/dev-postgres.sh env)"   # export QTF_DATABASE_DRIVER/DSN
#   scripts/dev-postgres.sh psql       # interactive shell
#   scripts/dev-postgres.sh stop|rm    # stop / delete container (+ data)
set -euo pipefail

PROG=$(basename "$0")
NAME=${QTF_PG_CONTAINER:-qtf-dev-postgres}
PORT=${QTF_PG_PORT:-55432}
IMAGE=${QTF_PG_IMAGE:-docker.io/library/postgres:16-alpine}
DB=qtestframe
USER_NAME=qtestframe
# Development only: fixed password, bound to localhost.
PASSWORD=${QTF_PG_PASSWORD:-qtestframe-dev}
PERSIST=${QTF_PG_PERSIST:-0}

usage() {
	cat <<EOF
Usage: $PROG start|stop|rm|status|logs|psql|dsn|env [--persist]

  start     create/start the container and wait until it accepts connections
  stop      stop the container (data kept unless the container is removed)
  rm        remove the container and its volume
  status    show container status
  logs      follow the server log
  psql      open psql inside the container
  dsn       print the DSN
  env       print 'export QTF_DATABASE_DRIVER=... QTF_DATABASE_DSN=...'

Options / environment:
  --persist             keep data in the named volume $NAME-data
                        (default: tmpfs, gone after stop)
  QTF_CONTAINER_ENGINE  podman or docker (default: autodetect, podman first)
  QTF_PG_PORT           host port (default $PORT)
  QTF_PG_IMAGE          image (default $IMAGE)
  QTF_PG_PASSWORD       password (default: $PASSWORD)
EOF
}

die() { printf '%s: error: %s\n' "$PROG" "$*" >&2; exit 1; }

CMD=${1:-}
[ -n "$CMD" ] || { usage >&2; exit 2; }
shift
while [ $# -gt 0 ]; do
	case "$1" in
	--persist) PERSIST=1 ;;
	-h | --help) usage; exit 0 ;;
	*) die "unknown option: $1" ;;
	esac
	shift
done

engine() {
	if [ -n "${QTF_CONTAINER_ENGINE:-}" ]; then
		echo "$QTF_CONTAINER_ENGINE"
	elif command -v podman >/dev/null 2>&1; then
		echo podman
	elif command -v docker >/dev/null 2>&1; then
		echo docker
	else
		die "neither podman nor docker found (macOS: brew install podman && podman machine init && podman machine start)"
	fi
}

dsn() {
	printf 'postgres://%s:%s@127.0.0.1:%s/%s?sslmode=disable' "$USER_NAME" "$PASSWORD" "$PORT" "$DB"
}

exists() { "$ENGINE" container inspect "$NAME" >/dev/null 2>&1; }
running() {
	[ "$("$ENGINE" container inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null || true)" = true ]
}

case "$CMD" in
-h | --help | help) usage; exit 0 ;;
dsn) dsn; echo; exit 0 ;;
env)
	printf 'export QTF_DATABASE_DRIVER=postgres\nexport QTF_DATABASE_DSN=%q\n' "$(dsn)"
	exit 0
	;;
esac

ENGINE=$(engine)

case "$CMD" in
start)
	if running; then
		echo "$NAME already running" >&2
	elif exists; then
		"$ENGINE" start "$NAME" >/dev/null
	else
		storage=(--tmpfs "/var/lib/postgresql/data:rw,size=1g")
		if [ "$PERSIST" = 1 ]; then storage=(-v "$NAME-data:/var/lib/postgresql/data"); fi
		"$ENGINE" run -d --name "$NAME" \
			-p "127.0.0.1:$PORT:5432" \
			-e POSTGRES_DB="$DB" -e POSTGRES_USER="$USER_NAME" -e POSTGRES_PASSWORD="$PASSWORD" \
			-e POSTGRES_INITDB_ARGS="--auth-host=scram-sha-256" \
			"${storage[@]}" \
			--label qtestframe=dev \
			"$IMAGE" >/dev/null
	fi
	printf 'waiting for postgres' >&2
	i=0
	until "$ENGINE" exec "$NAME" pg_isready -q -h 127.0.0.1 -U "$USER_NAME" -d "$DB" 2>/dev/null; do
		i=$((i + 1))
		[ "$i" -lt 60 ] || die "postgres did not become ready; see: $PROG logs"
		printf '.' >&2
		sleep 1
	done
	printf ' ready\n' >&2
	echo "DSN: $(dsn)" >&2
	echo "Use: eval \"\$($0 env)\"" >&2
	;;
stop) if exists; then "$ENGINE" stop "$NAME" >/dev/null; fi ;;
rm)
	if exists; then "$ENGINE" rm -f "$NAME" >/dev/null; fi
	"$ENGINE" volume rm "$NAME-data" >/dev/null 2>&1 || true
	;;
status)
	if exists; then "$ENGINE" ps -a --filter "name=^${NAME}\$"; else echo "$NAME does not exist"; fi
	;;
logs) "$ENGINE" logs -f "$NAME" ;;
psql) exec "$ENGINE" exec -it "$NAME" psql -U "$USER_NAME" -d "$DB" ;;
*) usage >&2; exit 2 ;;
esac
