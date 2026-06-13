#!/usr/bin/env bash
# bashfs profiling harness.
#
# This file is either embedded directly into a packaged script (--profiling-support
# local) or downloaded and eval'd at runtime (--profiling-support web). Either way
# it runs inside the packaged script's shell, with the bashfs runtime already set
# up, and is only reached when BASHFS_PROFILE_SCRIPT=1 is set in the environment.
#
# It expects these to be in scope (the generated bashfs block defines them):
#
#   __bashfs_self          path to the packaged script on disk (holds the payload)
#   __bashfs_payload_size  size, in bytes, of the trailing payload
#   __bashfs_decode        decode pipeline for one chunk (e.g. "gzip -d")
#   __bashfs_offset        associative array: relpath -> "offset:length"
#   __bashfs_payload_start function printing the 1-based payload start byte
#   bashfs_list            function printing every embedded path, sorted
#
# It uses hyperfine to benchmark the "parts" of the packaged script that bashfs
# is responsible for: the startup integrity check (sha256 over the whole payload,
# paid on every invocation before any user code runs) and the extraction of each
# embedded file (tail | head | decode). The output is a single hyperfine
# comparison table with a fastest/slowest summary.
#
# Optional environment knobs:
#   BASHFS_PROFILE_WARMUP   hyperfine --warmup count (default 3)
#   BASHFS_PROFILE_RUNS     hyperfine --runs count   (default: hyperfine's auto)
#   BASHFS_PROFILE_JSON     if set, also export raw results as JSON to this path

# __bashfs_profile_shq single-quotes a string so it can be embedded safely in a
# command line handed to hyperfine's shell, regardless of spaces or metacharacters.
__bashfs_profile_shq() {
  local s=$1
  s=${s//\'/\'\\\'\'}
  printf "'%s'" "$s"
}

__bashfs_profile_main() {
  if ! command -v hyperfine >/dev/null 2>&1; then
    echo "bashfs: profiling needs hyperfine, which was not found on PATH" >&2
    echo "bashfs: install it from https://github.com/sharkdp/hyperfine" >&2
    return 127
  fi

  local self=${__bashfs_self:-}
  if [ -z "$self" ] || [ ! -r "$self" ]; then
    echo "bashfs: profiling cannot read the packaged script (\"$self\")" >&2
    return 1
  fi

  local size=${__bashfs_payload_size:-0}
  if [ "$size" -le 0 ]; then
    echo "bashfs: profiling: empty embedded filesystem, nothing to benchmark" >&2
    return 1
  fi

  local decode=${__bashfs_decode:-gzip -d}
  local start
  start=$(__bashfs_payload_start)

  local warmup=${BASHFS_PROFILE_WARMUP:-3}
  local self_q
  self_q=$(__bashfs_profile_shq "$self")

  local -a args=(--warmup "$warmup")
  if [ -n "${BASHFS_PROFILE_RUNS:-}" ]; then
    args+=(--runs "$BASHFS_PROFILE_RUNS")
  fi
  if [ -n "${BASHFS_PROFILE_JSON:-}" ]; then
    args+=(--export-json "$BASHFS_PROFILE_JSON")
  fi

  # Part 1: the startup integrity check. Every run of the packaged script pays
  # this (sha256 over the whole payload) before any user code executes.
  args+=(-n "integrity-check"
         "tail -c $size $self_q | sha256sum >/dev/null")

  # Part 2: one extraction benchmark per embedded file. We rebuild the exact
  # tail | head | decode pipeline that bashfs_cat runs, so the numbers reflect
  # real per-file cost. bashfs_list gives us the paths already sorted, so the
  # table is stable across runs.
  local -a files=()
  local k
  while IFS= read -r k; do
    [ -n "$k" ] && files+=("$k")
  done < <(bashfs_list)

  for k in "${files[@]}"; do
    # __bashfs_offset is defined by the host bashfs block, not this file.
    # shellcheck disable=SC2154
    local info=${__bashfs_offset[$k]}
    local off=${info%%:*}
    local len=${info##*:}
    args+=(-n "cat:$k"
           "tail -c +$((start + off)) $self_q | head -c $len | $decode >/dev/null")
  done

  echo "bashfs: profiling ${#files[@]} embedded file(s) + integrity check from $self" >&2
  hyperfine "${args[@]}"
}

__bashfs_profile_main
exit $?
