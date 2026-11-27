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
# The long bot and realtime tests of internal/game (whole bot matches, a
# rocket ride per link pattern) run in a CI job of their own: in one go test
# run with the rest, the package passed CI's 10 minute test timeout.
# COVERAGE_PART picks what this run does:
#   all (default, local): every test, every floor.
#   main (CI job test): every test but the long and the slow ones, every floor but
#     internal/game's; writes internal/game's profile to cover-main.out.
#   ai (CI job test-ai, AI_SHARD moonshot, rest or slow): only the long ones,
#     TestAIMoonshot alone (~5 min) or the others (together they passed 9 min
#     on 4 cores), or the slow ones ($slow); writes cover-ai-$AI_SHARD.out. No floors.
#   merge (CI job coverage): internal/game's floor, from cover-main.out and
#     cover-ai-*.out (a statement counts as covered if any run covered it).
long='^(TestAI|TestRideRealtime|TestBotRuns)'
# The slowest of the rest (~2 min of the package's ~3 sequential: every sim/case
# through the server, delta vs json, server vs client, spawn and site searches):
# CI runs them as test-ai's third shard, beside test, to keep CI under 5 min.
slow='^(TestCasesDelta|TestCasesAgree|TestCases|TestSiteSpots|TestFullTeamsSpawnApart)$'
gamepkg=github.com/gigurra/counterslop/internal/game
part=${COVERAGE_PART:-all}
# go test's own timeout (default 10 min) times internal/game out on a loaded box;
# CI's job timeouts still bound the whole run.
tmo=${COVERAGE_TIMEOUT:-30m}
# pkgcover <profile...>: internal/game's statement coverage from go test profiles.
pkgcover() {
  grep -h "^$gamepkg/[^/]*:" "$@" | awk '{ if (!($1 in n)) { n[$1] = $2; c[$1] = 0 } if ($3 > 0) c[$1] = 1 }
    END { for (k in n) { t += n[k]; if (c[k]) h += n[k] } printf "%.1f\n", t ? 100 * h / t : 0 }'
}
case "$part" in
  all|main)
    args=(-count=1 -timeout "$tmo" -cover ./...)
    # MAIN_SHARD (CI): core = every package but internal/content/... (job test),
    # content = internal/content/... (job test-content: the map walks); each
    # checks its own packages' floors. Unset: every package.
    shard=${MAIN_SHARD:-}
    case "$shard" in
      "") pkgs=(./...) ;;
      core) mapfile -t pkgs < <(go list ./... | grep -v '/internal/content') ;;
      content) pkgs=(./internal/content/...) ;;
      *) echo "MAIN_SHARD: core or content"; exit 2 ;;
    esac
    [[ "$part" == main ]] && args=(-count=1 -timeout "$tmo" -cover -skip "$long|$slow" -coverprofile=cover-main.all "${pkgs[@]}")
    # CI runs the tests only here (once, with coverage): a failing test fails the script.
    go test "${args[@]}" > "$raw" 2>&1 || { echo "go test failed"; fail=1; }
    sed -nE 's#^(ok|---)?[[:space:]]*github.com/gigurra/counterslop/?([^[:space:]]*)[[:space:]].*coverage: ([0-9.]+)% of statements.*#\2 \3#p' "$raw" > "$out"
    if [[ "$part" == main && "$shard" != content ]]; then
      { echo "mode: set"; grep -h "^$gamepkg/[^/]*:" cover-main.all; } > cover-main.out
      sed -i '/^internal\/game /d' "$out" # measured across both runs: COVERAGE_PART=merge
    fi
    rm -f cover-main.all
    # A failing test reads as 0% coverage below: show which one, or the log hides it.
    grep -E -- '^(--- FAIL|FAIL|panic:)|_test\.go:[0-9]+:' "$raw" | head -200
    if [[ "${shard:-}" != content ]]; then
      js=$(node --test --experimental-test-coverage --test-coverage-exclude='**/*.test.mjs' 'web/**/*.test.mjs' 2>&1 | sed -nE 's/^ℹ all files +\| +([0-9.]+) .*/\1/p')
      echo "web(js) ${js:-0}" >> "$out"
    fi
    ;;
  ai)
    case "${AI_SHARD:-}" in
      moonshot) sel=(-run '^TestAIMoonshot$') ;;
      rest) sel=(-run "$long" -skip '^TestAIMoonshot$') ;;
      slow) sel=(-run "$slow") ;;
      *) echo "AI_SHARD: moonshot, rest or slow"; exit 2 ;;
    esac
    go test -count=1 -timeout "$tmo" "${sel[@]}" -coverprofile="cover-ai-$AI_SHARD.out" ./internal/game 2>&1 | tee "$raw" | grep -E -- '^(--- FAIL|FAIL|panic:|ok)|_test\.go:[0-9]+:' | head -200
    [[ ${PIPESTATUS[0]} -eq 0 ]] || { echo "go test failed"; fail=1; }
    rm -f "$raw" "$out"
    exit $fail
    ;;
  merge)
    for f in cover-main.out cover-ai-moonshot.out cover-ai-rest.out cover-ai-slow.out; do [[ -s "$f" ]] || { echo "coverage: no $f"; exit 1; }; done
    echo "internal/game $(pkgcover cover-main.out cover-ai-moonshot.out cover-ai-rest.out cover-ai-slow.out)" > "$out"
    ;;
  *) echo "COVERAGE_PART: all, main, ai or merge (got $part)"; exit 2 ;;
esac
rm -f "$raw"
md="### Coverage\n\n| package | coverage | floor | |\n|---|---|---|---|\n"
while read -r pkg floor; do
  [[ -z "$pkg" || "$pkg" == \#* ]] && continue
  key=$pkg; [[ "$pkg" == "." ]] && key=""
  # A main shard checks its own packages' floors only (the other shard checks the rest).
  if [[ "$part" == main && -n "${shard:-}" ]]; then
    case "$pkg" in internal/content*) [[ "$shard" == content ]] || continue ;; *) [[ "$shard" == core ]] || continue ;; esac
  fi
  got=$(awk -v p="$key" '$1==p {print $2}' "$out")
  if [[ -z "$got" ]]; then
    [[ "$part" == main && "$pkg" == internal/game ]] && continue # the coverage job checks it
    [[ "$part" == merge ]] && continue                          # only internal/game here
  fi
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
