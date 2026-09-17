#!/usr/bin/env bash
# Verify that Caddy's JSON access-log shape still matches what GoAccess expects.
#
# Usage:
#   ./scripts/test-goaccess-format.sh                    # run against the built-in fixture
#   ./scripts/test-goaccess-format.sh /path/to/access.log # run against a real log
#
# Why this exists: the goaccess service in docker-compose.yml parses Caddy's log
# with `--log-format=CADDY --date-format=%s --time-format=%s`. The two %s flags
# are load-bearing — Caddy's JSON encoder writes `ts` as a Unix epoch float
# ("unix_seconds_float"), and GoAccess's default date parsing would match zero
# lines against it. The failure is SILENT: GoAccess exits 0 and writes a report
# with no data in it, so nothing alerts you.
#
# Run this after any Caddy major bump, or whenever the dashboard looks empty.
set -euo pipefail

log_file="${1:-}"
fixture=""

if [ -z "$log_file" ]; then
	fixture="$(mktemp)"
	# shellcheck disable=SC2064
	trap "rm -f '$fixture'" EXIT
	cat > "$fixture" <<'EOF'
{"level":"info","ts":1757942400.123456,"logger":"http.log.access","msg":"handled request","request":{"remote_ip":"203.0.113.45","remote_port":"54321","client_ip":"203.0.113.45","proto":"HTTP/2.0","method":"GET","host":"visoto.hutzli.org","uri":"/resource?iri=https://ld.admin.ch/canton/1","headers":{"User-Agent":["Mozilla/5.0 (X11; Linux x86_64) Chrome/140"],"Referer":["https://visoto.hutzli.org/"]}},"bytes_read":0,"user_id":"","duration":0.412345,"size":48213,"status":200}
{"level":"info","ts":1757942401.987654,"logger":"http.log.access","msg":"handled request","request":{"remote_ip":"198.51.100.7","remote_port":"41234","client_ip":"198.51.100.7","proto":"HTTP/2.0","method":"GET","host":"visoto.hutzli.org","uri":"/static/img/monitor.svg","headers":{"User-Agent":["Mozilla/5.0 (X11; Linux x86_64) Chrome/140"]}},"bytes_read":0,"user_id":"","duration":0.001234,"size":1422,"status":200}
{"level":"info","ts":1757942402.500000,"logger":"http.log.access","msg":"handled request","request":{"remote_ip":"172.18.0.4","remote_port":"33001","client_ip":"172.18.0.4","proto":"HTTP/1.1","method":"GET","host":"visoto.hutzli.org","uri":"/api/metric?q=x","headers":{"User-Agent":["curl/8.5.0"]}},"bytes_read":0,"user_id":"","duration":12.75,"size":312,"status":200}
EOF
	log_file="$fixture"
	echo "No log file given — using the built-in fixture."
fi

echo "Checking: $log_file"
echo

python3 - "$log_file" <<'PY'
import json, sys

path = sys.argv[1]
# Fields the CADDY log-format reads. If Caddy stops emitting one of these, the
# corresponding GoAccess panel goes quietly empty.
required_top = ["ts", "status", "size", "duration"]
required_req = ["client_ip", "method", "uri", "proto"]

checked = skipped = 0
failures = []

with open(path) as fh:
    for lineno, line in enumerate(fh, 1):
        line = line.strip()
        if not line:
            continue
        try:
            entry = json.loads(line)
        except json.JSONDecodeError as exc:
            failures.append(f"line {lineno}: not valid JSON ({exc})")
            continue

        # Real logs interleave non-access entries; only access lines are parsed.
        if entry.get("logger") != "http.log.access":
            skipped += 1
            continue
        checked += 1

        for field in required_top:
            if field not in entry:
                failures.append(f"line {lineno}: missing top-level {field!r}")

        req = entry.get("request")
        if not isinstance(req, dict):
            failures.append(f"line {lineno}: missing 'request' object")
        else:
            for field in required_req:
                if field not in req:
                    failures.append(f"line {lineno}: missing request.{field}")

        # The load-bearing assertion.
        ts = entry.get("ts")
        if not isinstance(ts, (int, float)) or isinstance(ts, bool):
            failures.append(
                f"line {lineno}: ts is {type(ts).__name__} ({ts!r}), expected a "
                "numeric Unix epoch. Caddy's log format changed — the "
                "--date-format=%s / --time-format=%s flags in docker-compose.yml "
                "are no longer correct and GoAccess will parse 0 lines."
            )

if not checked:
    print("FAIL: no http.log.access entries found — nothing was verified.")
    if skipped:
        print(f"      ({skipped} non-access lines skipped)")
    sys.exit(1)

print(f"Access-log entries checked: {checked}" + (f" ({skipped} non-access skipped)" if skipped else ""))

if failures:
    print("\nFAIL:")
    for f in failures:
        print(f"  - {f}")
    sys.exit(1)

print("PASS: ts is a numeric Unix epoch on every access entry,")
print("      and every field the CADDY log-format needs is present.")
print("      => --date-format=%s --time-format=%s remain correct.")
PY
