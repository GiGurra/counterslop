#!/usr/bin/env bash
# Coverage ratchet: per-package Go statement coverage and the JS unit tests' line
# coverage must stay at or above the floors in coverage-floors.txt. Raise a floor
# when you raise coverage; never lower one to make CI pass (fix the tests).
# Writes a markdown table to $GITHUB_STEP_SUMMARY when set.
set -uo pipefail
cd "$(dirname "$0")/.."
floors=coverage-floors.txt
out=$(mktemp)
fail=0
raw=$(mktemp)
go test -count=1 -cover ./... > "$raw" 2>&1
sed -nE 's#^(ok|---)?[[:space:]]*github.com/gigurra/counterslop/?([^[:space:]]*)[[:space:]].*coverage: ([0-9.]+)% of statements.*#\2 \3#p' "$raw" > "$out"
# A failing test reads as 0% coverage below: show which one, or the log hides it.
grep -E -- '^(--- FAIL|FAIL|panic:)|_test\.go:[0-9]+:' "$raw" | head -40
rm -f "$raw"
js=$(node --test --experimental-test-coverage --test-coverage-exclude='**/*.test.mjs' 'web/**/*.test.mjs' 2>&1 | sed -nE 's/^ℹ all files +\| +([0-9.]+) .*/\1/p')
echo "web(js) ${js:-0}" >> "$out"
md="### Coverage\n\n| package | coverage | floor | |\n|---|---|---|---|\n"
while read -r pkg floor; do
  [[ -z "$pkg" || "$pkg" == \#* ]] && continue
  key=$pkg; [[ "$pkg" == "." ]] && key=""
  got=$(awk -v p="$key" '$1==p {print $2}' "$out")
  got=${got:-0}
  if awk -v g="$got" -v f="$floor" 'BEGIN{exit !(g+0 < f+0)}'; then
    md+="| $pkg | $got% | $floor% | ❌ below the floor |\n"; fail=1
    echo "coverage: $pkg $got% < floor $floor%"
  else
    md+="| $pkg | $got% | $floor% | ✅ |\n"
  fi
done < "$floors"
printf "%b" "$md"
[[ -n "${GITHUB_STEP_SUMMARY:-}" ]] && printf "%b" "$md" >> "$GITHUB_STEP_SUMMARY"
rm -f "$out"
exit $fail
