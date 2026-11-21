#!/bin/sh
# A stand-in for `playwright test --config <file>` in tests (no browser
# needed). It runs in the job directory and acts on words in the spec:
#   SLEEP     sleeps 5 s (timeouts, cancelling)
#   NOREPORT  exits without a report
#   FAIL      fails with a screenshot and a trace (and a file outside)
#   SKIP      reports a skipped test
# Otherwise the test passes. It prints the variables it was given.
[ "$1" = test ] && [ "$2" = --config ] && [ -f "$3" ] || { echo "usage: test --config <file>" >&2; exit 9; }
spec=test.spec.ts
echo "base=$QTF_BASE_URL browser=$QTF_BROWSER channel=$QTF_BROWSER_CHANNEL token=$QTF_VAR_TOKEN user=$QTF_VAR_USERNAME"
grep -q "baseURL" "$3" && echo "config has baseURL"
if grep -q SLEEP $spec; then sleep 5; fi
if grep -q NOREPORT $spec; then echo "crashed"; exit 2; fi
mkdir -p results/t
if grep -q FAIL $spec; then
  printf 'PNG-DATA' > results/t/test-failed-1.png
  printf 'ZIP-DATA' > results/t/trace.zip
  printf 'VIDEO' > results/t/video.webm
  cat > report.json <<EOF
{"config":{},"suites":[{"title":"test.spec.ts","specs":[{"title":"login","tests":[{"status":"unexpected","results":[{"status":"failed",
 "error":{"message":"\u001b[31mExpected heading, got s3cr3t-token\u001b[39m"},
 "attachments":[{"name":"screenshot","contentType":"image/png","path":"$PWD/results/t/test-failed-1.png"},
  {"name":"trace","contentType":"application/zip","path":"$PWD/results/t/trace.zip"},
  {"name":"video","contentType":"video/webm","path":"$PWD/results/t/video.webm"},
  {"name":"screenshot","contentType":"image/png","path":"/etc/passwd"},
  {"name":"error-context","contentType":"text/markdown","path":"$PWD/results/t/error-context.md"}]}]}]}]}],
 "errors":[],"stats":{"expected":0,"unexpected":1}}
EOF
  echo "  1 failed: Expected heading, got s3cr3t-token"
  exit 1
fi
if grep -q SKIP $spec; then
  echo '{"config":{},"suites":[{"title":"test.spec.ts","specs":[{"title":"s","tests":[{"status":"skipped","results":[{"status":"skipped","attachments":[]}]}]}]}],"errors":[]}' > report.json
  exit 0
fi
echo '{"config":{},"suites":[{"title":"test.spec.ts","specs":[{"title":"ok","tests":[{"status":"expected","results":[{"status":"passed","attachments":[]}]}]}]}],"errors":[],"stats":{"expected":1}}' > report.json
echo "  1 passed"
exit 0
