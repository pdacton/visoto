#!/usr/bin/env bash
# Find running visoto instances (and what port each is listening on), optionally kill them.
#
# Usage:
#   ./scripts/visoto-ps.sh                 # list only
#   ./scripts/visoto-ps.sh --kill          # kill all except the port-8060 dev server
#   ./scripts/visoto-ps.sh --kill --all    # kill everything, including port 8060
#   ./scripts/visoto-ps.sh --kill --port 8061
#
# By default port 8060 is protected: it is the human's dev server. --all overrides.
set -euo pipefail

protected_port=8060
do_kill=0
kill_all=0
only_port=""

while [ $# -gt 0 ]; do
	case "$1" in
		--kill|-k) do_kill=1 ;;
		--all|-a) kill_all=1 ;;
		--port|-p)
			shift
			[ $# -gt 0 ] || { echo "Error: --port needs a value" >&2; exit 1; }
			only_port="$1"
			;;
		-h|--help)
			sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
			exit 0
			;;
		*) echo "Error: unknown argument '$1' (try --help)" >&2; exit 1 ;;
	esac
	shift
done

# Match both a built binary (./visoto, cmd/visoto/visoto) and `go run ./cmd/visoto/`,
# which executes a temp binary under /tmp/go-build*/b001/exe/visoto.
mapfile -t pids < <(pgrep -f '(^|/)visoto( |$)|/exe/visoto|go run .*cmd/visoto' 2>/dev/null || true)

# Drop this script and any pgrep/grep helpers that matched their own command line.
filtered=()
for pid in "${pids[@]:-}"; do
	[ -n "$pid" ] || continue
	[ "$pid" = "$$" ] && continue
	cmd=$(ps -p "$pid" -o args= 2>/dev/null || true)
	[ -n "$cmd" ] || continue
	case "$cmd" in
		*visoto-ps.sh*) continue ;;
	esac
	filtered+=("$pid")
done

if [ ${#filtered[@]} -eq 0 ]; then
	echo "No visoto processes running."
	exit 0
fi

# Resolve the listening port per pid. ss is present on WSL; fall back to lsof.
port_for_pid() {
	local pid="$1" port=""
	if command -v ss >/dev/null 2>&1; then
		port=$(ss -tlnpH 2>/dev/null | grep -o "[0-9.:*\[\]]*:\([0-9]*\)[^;]*pid=$pid," \
			| sed 's/.*:\([0-9]*\) .*/\1/' | head -1)
	fi
	if [ -z "$port" ] && command -v lsof >/dev/null 2>&1; then
		port=$(lsof -Pan -p "$pid" -iTCP -sTCP:LISTEN -Fn 2>/dev/null \
			| sed -n 's/^n.*:\([0-9]*\)$/\1/p' | head -1)
	fi
	printf '%s' "${port:-?}"
}

# `go run` spawns the real server as a child, and only that child holds the socket.
# The wrapper therefore has no port of its own -- inherit the child's, so the
# protected-port guard covers the whole process tree rather than just the listener.
effective_port() {
	local pid="$1" port
	port=$(port_for_pid "$pid")
	if [ "$port" = "?" ]; then
		local child
		for child in $(pgrep -P "$pid" 2>/dev/null || true); do
			local child_port
			child_port=$(effective_port "$child")
			if [ "$child_port" != "?" ]; then
				printf '%s' "$child_port"
				return
			fi
		done
	fi
	printf '%s' "$port"
}

printf '%-8s %-6s %s\n' PID PORT COMMAND
targets=()
for pid in "${filtered[@]}"; do
	port=$(effective_port "$pid")
	cmd=$(ps -p "$pid" -o args= 2>/dev/null | cut -c1-70)
	printf '%-8s %-6s %s\n' "$pid" "$port" "$cmd"

	if [ -n "$only_port" ] && [ "$port" != "$only_port" ]; then
		continue
	fi
	if [ "$port" = "$protected_port" ] && [ "$kill_all" -eq 0 ]; then
		continue
	fi
	targets+=("$pid:$port")
done

[ "$do_kill" -eq 1 ] || { echo; echo "(list only — pass --kill to terminate)"; exit 0; }

if [ ${#targets[@]} -eq 0 ]; then
	echo
	echo "Nothing to kill (port $protected_port is protected; use --all to include it)."
	exit 0
fi

echo
# Killing a `go run` wrapper orphans the server it spawned, so take children too.
expanded=()
for entry in "${targets[@]}"; do
	pid="${entry%%:*}"
	port="${entry##*:}"
	expanded+=("$entry")
	for child in $(pgrep -P "$pid" 2>/dev/null || true); do
		case " ${targets[*]} " in
			*" $child:"*) continue ;;
		esac
		expanded+=("$child:$port")
	done
done
targets=("${expanded[@]}")

for entry in "${targets[@]}"; do
	pid="${entry%%:*}"
	port="${entry##*:}"
	echo "Terminating pid $pid (port $port)..."
	kill "$pid" 2>/dev/null || true
done

# Give them a moment to exit cleanly, then escalate to SIGKILL for stragglers.
for _ in 1 2 3 4 5 6 7 8 9 10; do
	still_running=0
	for entry in "${targets[@]}"; do
		kill -0 "${entry%%:*}" 2>/dev/null && still_running=1
	done
	[ "$still_running" -eq 0 ] && break
	sleep 0.2
done

for entry in "${targets[@]}"; do
	pid="${entry%%:*}"
	if kill -0 "$pid" 2>/dev/null; then
		echo "pid $pid ignored SIGTERM, sending SIGKILL"
		kill -9 "$pid" 2>/dev/null || true
	fi
done

echo "Done."
