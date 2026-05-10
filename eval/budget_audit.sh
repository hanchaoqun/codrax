#!/usr/bin/env bash
#
# eval/budget_audit.sh — post-sweep budget exhaustion + L3/L4 usage audit.
#
# Reads every result dir from the most recent sweep (filtered by
# the SWEEP_PREFIX env var, default = today) and extracts:
#
#   1. Analyzer prescan budget exhaustion frequency
#      (counts cases where "Pre-scan budget reached" fires)
#   2. Explorer iter usage per dispatch
#      (analyzer_iters, explorer_iters from run-1.metrics.txt)
#   3. Stage dispatch counts (analyzer/explorer/extractor/finalizer)
#   4. L3 RequiredFileHints production: how often LLM populates them
#   5. L4 IrrelevantFiles production: how often LLM populates them
#   6. L3 graph-validation drops: how often hallucinated paths get culled
#   7. Pipeline-wide max-steps exhaustion (orchestrator forcing stop)
#
# Output is markdown to stdout, suitable for piping into a report.
#
# Usage:
#   bash eval/budget_audit.sh                       # today's sweep
#   SWEEP_PREFIX=20260510 bash eval/budget_audit.sh # specific date
#
# 2026-05-10 — added after L1-L4 forced-read remediation to track
# whether the prescan-budget bump (2→3) is enough or needs further
# raising, plus give visibility on L3/L4 typed-channel adoption.

set -uo pipefail

SWEEP_PREFIX="${SWEEP_PREFIX:-$(date +%Y%m%d)}"
# Result dir format is `<case>-<YYYYMMDD>-<HHMMSS>` so we glob with
# the prefix as an embedded substring. Wildcards on both sides keep
# this tolerant to operator-supplied partial timestamps
# (e.g. `SWEEP_PREFIX=20260510-08` to scope to today's afternoon).
shopt -s nullglob
SWEEP_DIRS=(eval/results/*-${SWEEP_PREFIX}*)
shopt -u nullglob

if [[ ${#SWEEP_DIRS[@]} -eq 0 ]]; then
  echo "no result dirs found matching prefix ${SWEEP_PREFIX}" >&2
  exit 1
fi

# --- pass 1: collect ---
declare -i total=0
declare -i prescan_exhausted=0
declare -i l3_emitted=0
declare -i l3_dropped_by_graph=0
declare -i l4_emitted=0
declare -i pipeline_step_exhausted=0
declare -A explorer_iters_hist
declare -A analyzer_iters_hist

# Per-case rows
PER_CASE=()

for dir in "${SWEEP_DIRS[@]}"; do
  [[ -d "$dir" ]] || continue
  log=$(ls "$dir"/run-1.logs/*.log 2>/dev/null | head -1)
  metrics="$dir/run-1.metrics.txt"
  verdict_file="$dir/run-1.verdict"
  [[ -z "$log" ]] && continue
  total+=1

  # Strip the trailing `-YYYYMMDD-HHMMSS` timestamp suffix to recover
  # the case_id. Use a fixed-width regex so this works regardless of
  # the SWEEP_PREFIX granularity.
  case_id=$(basename "$dir" | sed -E 's/-[0-9]{8}-[0-9]{6}$//')

  # Verdict (PASS/FAIL/TIMEOUT/...)
  verdict=$(head -1 "$verdict_file" 2>/dev/null | awk '{print $1}')
  verdict="${verdict:-UNKNOWN}"

  # Prescan budget exhaustion. We count GRACE-round triggers
  # (one per analyzer dispatch that hit the ceiling), not log-echo
  # repetitions. The MUST-emit hint message fires once per actual
  # exhaustion event.
  prescan_ex=$(awk 'index($0,"must-emit hint built")>0 && index($0,"Pre-scan budget reached")>0{n++} END{print n+0}' "$log" 2>/dev/null)
  if [[ ${prescan_ex:-0} -gt 0 ]]; then
    prescan_exhausted+=1
  fi

  # L3 emit + graph drops.
  l3_emit=$(awk 'index($0,"required_file_hints: emitted=")>0{
    if (match($0, /emitted=([0-9]+) /, m)) print m[1]; exit
  }' "$log" 2>/dev/null)
  l3_emit="${l3_emit:-0}"
  if [[ "$l3_emit" -gt 0 ]]; then
    l3_emitted+=1
  fi
  l3_drop=$(awk 'index($0,"dropped_by_graph=")>0{
    if (match($0, /dropped_by_graph=([0-9]+)/, m)) print m[1]; exit
  }' "$log" 2>/dev/null)
  l3_drop="${l3_drop:-0}"
  if [[ "$l3_drop" -gt 0 ]]; then
    l3_dropped_by_graph+=1
  fi

  # L4 emit.
  l4_emit=$(awk 'index($0,"irrelevant_files: declared=")>0{
    if (match($0, /declared=([0-9]+)/, m)) print m[1]; exit
  }' "$log" 2>/dev/null)
  l4_emit="${l4_emit:-0}"
  if [[ "$l4_emit" -gt 0 ]]; then
    l4_emitted+=1
  fi

  # Pipeline max-steps exhaustion (orchestrator-level).
  pipe_ex=$(awk 'index($0,"step budget exhausted")>0 || index($0,"max_steps reached")>0{n++} END{print n+0}' "$log" 2>/dev/null)
  if [[ ${pipe_ex:-0} -gt 0 ]]; then
    pipeline_step_exhausted+=1
  fi

  # Per-stage iters (from metrics.txt).
  ana_iters=0
  exp_iters=0
  ext_iters=0
  fin_iters=0
  if [[ -f "$metrics" ]]; then
    ana_iters=$(grep -oE 'analyzer_iters=[0-9]+' "$metrics" | head -1 | cut -d= -f2)
    exp_iters=$(grep -oE 'explorer_iters=[0-9]+' "$metrics" | head -1 | cut -d= -f2)
    ext_iters=$(grep -oE 'extractor_iters=[0-9]+' "$metrics" | head -1 | cut -d= -f2)
    fin_iters=$(grep -oE 'finalizer_iters=[0-9]+' "$metrics" | head -1 | cut -d= -f2)
  fi

  # Histograms
  analyzer_iters_hist[${ana_iters:-0}]=$((${analyzer_iters_hist[${ana_iters:-0}]:-0} + 1))
  explorer_iters_hist[${exp_iters:-0}]=$((${explorer_iters_hist[${exp_iters:-0}]:-0} + 1))

  # Per-case row
  PER_CASE+=("$(printf '| %-25s | %-6s | %-3s | %-3s | %-3s | %-3s | %-3s | %-3s | %-3s |' \
    "$case_id" "$verdict" "$prescan_ex" "$l3_emit" "$l3_drop" "$l4_emit" \
    "${ana_iters:-0}" "${exp_iters:-0}" "${ext_iters:-0}")")
done

# --- pass 2: report ---
cat <<EOF
# Budget + Typed-Channel Audit (sweep prefix: ${SWEEP_PREFIX})

## Headline metrics

| Metric | Count | % of $total |
|---|---:|---:|
| Total cases analysed | $total | 100% |
| Prescan budget exhausted | $prescan_exhausted | $(printf '%.1f' "$(echo "scale=3; 100 * $prescan_exhausted / $total" | bc 2>/dev/null)")% |
| Pipeline step-budget exhausted | $pipeline_step_exhausted | $(printf '%.1f' "$(echo "scale=3; 100 * $pipeline_step_exhausted / $total" | bc 2>/dev/null)")% |
| L3 \`required_files\` emitted | $l3_emitted | $(printf '%.1f' "$(echo "scale=3; 100 * $l3_emitted / $total" | bc 2>/dev/null)")% |
| L3 graph-validator dropped paths | $l3_dropped_by_graph | $(printf '%.1f' "$(echo "scale=3; 100 * $l3_dropped_by_graph / $total" | bc 2>/dev/null)")% |
| L4 \`irrelevant_files\` emitted | $l4_emitted | $(printf '%.1f' "$(echo "scale=3; 100 * $l4_emitted / $total" | bc 2>/dev/null)")% |

## Analyzer iters histogram

| iters | count |
|---:|---:|
EOF
for k in $(echo "${!analyzer_iters_hist[@]}" | tr ' ' '\n' | sort -n); do
  printf "| %s | %s |\n" "$k" "${analyzer_iters_hist[$k]}"
done

cat <<EOF

## Explorer iters histogram

| iters | count |
|---:|---:|
EOF
for k in $(echo "${!explorer_iters_hist[@]}" | tr ' ' '\n' | sort -n); do
  printf "| %s | %s |\n" "$k" "${explorer_iters_hist[$k]}"
done

cat <<EOF

## Per-case detail

| case | verdict | prescan_ex | l3_emit | l3_drop | l4_emit | ana_it | exp_it | ext_it |
|------|---------|----------:|--------:|--------:|--------:|-------:|-------:|-------:|
EOF
printf '%s\n' "${PER_CASE[@]}"

cat <<EOF

## Decision tree (post-2026-05-10 forced-read remediation)

- **Prescan budget exhausted % ≥ 30%** → consider raising default 3→4 (currently 3 after B0 commit 78110a9).
- **Pipeline step-budget exhausted % > 5%** → indicates real timeouts that need bigger per-stage caps.
- **L3 emitted % < 20%** → analyzer LLM not adopting the new typed channel; consider strengthening skill prompt or adding a few-shot example.
- **L3 graph-dropped % > 5%** → LLM is hallucinating paths despite prescan; consider tightening rationale validation or adding negative training in the skill.
- **L4 emitted % < 5%** → expected for the first weeks; the negative channel is a low-base-rate signal that grows with operator usage.
EOF
