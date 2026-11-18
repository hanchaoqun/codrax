# HMC batch tools (`eval/hmc_tools/`)

Five small Python 3 programs (standard library only, Python >= 3.9) that replace the
out-of-repo Ruby scripts every HMC batch used to carry in `/private/tmp`
(`codrax-hmcNNN-batch-v1.rb`, `-close-v1.rb`, `-wrapper-v1`, `-restore-baselines-v1.rb`).
They produce the numbers a batch report quotes and nothing else: they do not call a
model, do not read `providers.yaml`, do not read a question or an answer to decide
anything, and do not write a verdict.

| Tool | Report line it owns |
|---|---|
| `freeze.py` | `冻结 SHA256 <hex>：7,704 源输入、78 外部、12 变更` |
| `suite_stats.py` | `唯一全仓 exit 1，97 包通过、12 无测试、1 失败；28,819 顶层、34,731 子通过，重复/未终态 0` and `定向 5 包 60/120` |
| `launch.py` | "两例各一次" (two cases, once each, one start barrier, `--root-causes-out` for the read case) |
| `overlap.py` | `实际模型请求 A13/B24，25 对重叠、最长 14.948 秒，歧义 0` |
| `seal.py` | `封存 261 件，索引 SHA256 <hex>，收据 <hex>；前代 261 件逐份复验` |

`hmc_common.py` holds the shared rules; `hmc_tools_test.py` is the zero-model self-test
(`python3 eval/hmc_tools/hmc_tools_test.py`, 12 tests, about 15 s).

## Rules every tool obeys

1. **Evidence is created once.** Every output file is opened with `O_EXCL`; a second run
   with the same label, manifest path, batch root or seal generation is a refusal
   (exit 2), never an overwrite. A failed result is kept as it is.
2. **No credentials.** Paths with a `providers*` or `.env*` component are refused for
   hashing, copying and output. The only exception is a *tracked* source file with such
   a name (`internal/config/providers.go`), which is ordinary source.
3. **No symlinks** where bytes are hashed or copied.
4. **Nothing is written into the repository working tree** except under the git-ignored
   batch root `eval/results/hmc_<topic>_<date>_v1/` (launch results and the seal).
   Check logs, manifests and snapshots live in a work directory outside the repository.
   Do not use `/private/tmp` for it: the system cleans it and the §375 inputs were lost
   that way.
5. **Exit codes:** `0` done and clean, `1` done but the recorded result is not clean
   (test failure, changed input, mismatch, timeout), `2` refused before doing anything.

## One batch, in order

```sh
REPO=/abs/path/to/codrax                    # the checkout being evaluated
T=$REPO/eval/hmc_tools
W=$HOME/hmc-work/hmc376                     # evidence work dir, outside the repo, persistent
ROOT=$REPO/eval/results/hmc_<topic>_<YYYYMMDD>_v1
PREV=/abs/path/to/eval/results/hmc_repair_generation_contract_20261111_v1   # previous batch root
BASE=$(git -C $REPO rev-parse HEAD)         # local main before the candidate is committed
mkdir -p $W
```

### 0. Re-verify the previous batch (read-only)

```sh
python3 $T/seal.py verify --root $PREV
# {"count": 261, "verified": 261, "index_sha256": "c32dfe4a…7e80", "receipt_sha256": "f4d53bee…1a64", "all_verified": true, …}
```

Inputs of env-gated regressions that only survive in a sealed archive are copied back out
with `restore` (it never writes into a sealed directory, never overwrites):

```sh
python3 $T/seal.py restore --root /abs/eval/results/hmc_tree_depth_restore_20261108_v1 \
  --source /private/tmp/codrax-hmc352-whole-inputs-v1.json --to $W/hmc352-whole-inputs-v1.json
```

### 1. Targeted, adjacent, race, whole-original and build checks

One call per check; each writes `<label>-intent.json`, `<label>.log`, `<label>-receipt.json`
and, for `go test -json`, `<label>-stats.json` into `$W`. The command after `--` is run
from `$REPO` with stdin closed and with the operator's `CODRAX_*`, `EXPECT_*`, `LIVE_*`,
`MAX_*` and run.sh case variables removed; add what the check needs with `--env`.

```sh
python3 $T/suite_stats.py run --label focused-v1 --out-dir $W -- \
  go test -json ./internal/agent ./internal/tool -run 'TestHMC376' -count=1
python3 $T/suite_stats.py run --label race-v1 --out-dir $W -- \
  go test -json -race -count=2 ./internal/agent -run 'TestHMC376'
python3 $T/suite_stats.py run --label whole-v1 --out-dir $W \
  --env CODRAX_HMC365_HONEYBIRD=/abs/ftrace-20261019_19.24.35.ftrace -- \
  go test -json ./internal/tool -run 'TestHMC365HoneybirdWholeNativeWaitSequence' -count=1
python3 $T/suite_stats.py run --label build-v1 --out-dir $W --raw -- make
cp $REPO/codrax $W/codrax-cli-v1            # the frozen CLI copy the two cases will run
```

Each prints the brief JSON, then `exit 0，N 包通过、…` and `N 包 T/S` (the form
"定向 5 包 60/120" uses), then one `FAIL` / `DUPLICATE` / `NON-TERMINAL` line per offender.
A skipped env-gated test shows up in `top_skip`; a skip is not a pass.

### 2. Freeze

```sh
python3 $T/freeze.py write --repo $REPO --base $BASE --branch main --out $W/frozen-v1.json \
  --snapshot-dir $W/snapshots-v1 \
  --external /abs/ftrace-20261019_19.24.35.ftrace \
  --external $W/codrax-cli-v1 \
  --external $PREV/sealed-index-v1.json --external $PREV/sealed-receipt-v1.json --external $PREV/release-v1.json \
  --external-list $W/reference-files.txt \
  --case A=$REPO/eval/cases/real_traces/donghu_v105_honeybird_startup_wait.case \
  --case B=$REPO/eval/cases/native_registration_commandless.case \
  --require-receipt $W/focused-v1-receipt.json --require-receipt $W/race-v1-receipt.json \
  --require-receipt $W/whole-v1-receipt.json --require-receipt $W/build-v1-receipt.json
# {"freeze_sha256": "…", "inputs": 7704, "external": 78, "changed": 12}
# 冻结 SHA256 …：7,704 源输入、78 外部、12 变更
```

* `inputs` is every tracked or untracked-not-ignored file under `internal/ cmd/ eval/cases/
  eval/fixtures/ eval/lib/ third_party/ examples/ config/ scripts/`, plus any
  `.go .sh .py .yaml .yml .json .toml` file and `Makefile go.mod go.sum`, minus `docs/`,
  `eval/results/`, `.codrax/`. These tools are therefore frozen too once they are in the repo.
* `changed` is the working tree against `--base` (committed, staged, unstaged, untracked).
  A changed file that is not a source input (a doc, a report) stops the freeze unless
  `--allow-changed-outside-inputs` is given; such files are listed separately and not counted.
* `--require-receipt` accepts only a green receipt taken on exactly these source bytes.
* After this command nothing is edited. `python3 $T/freeze.py verify --manifest $W/frozen-v1.json`
  re-hashes everything and prints each difference (exit 1).

The old Ruby rule skipped three tracked files by name (`internal/config/providers.go`,
`internal/types/providers.go`, `providers.yaml.example`); they are frozen now and listed in
`legacy_name_excluded`, which is why HEAD `1aceb24f3` counts 7,704 where §375 reported 7,701.
`verify` still accepts the old manifests (`--source-only` when their `/private/tmp`
externals are gone).

### 3. The single full-repo run

```sh
python3 $T/suite_stats.py run --label full-v1 --out-dir $W --freeze $W/frozen-v1.json
```

With no command this runs exactly `go test -json ./... -count=1 -timeout=30m`, once. The
label can never be reused, so a red result cannot be replaced by a later green one. It may
run while step 4 is running. To recount an existing log:
`python3 $T/suite_stats.py read --log <full.log> --exit <code>`.

### 4. Launch the two cases

```sh
python3 $T/launch.py run --repo $REPO --root $ROOT \
  --case A=$REPO/eval/cases/real_traces/donghu_v105_honeybird_startup_wait.case \
  --case B=$REPO/eval/cases/native_registration_commandless.case \
  --bin $W/codrax-cli-v1 \
  --providers-args "--providers $REPO/providers.yaml" \
  --env A:CODRAX_HMC365_HONEYBIRD=/abs/ftrace-20261019_19.24.35.ftrace \
  --freeze $W/frozen-v1.json --timeout 1800
```

Result layout:

```
$ROOT/A/<case-id>-<ts>/run-1.*            eval/run.sh output of case A (+ run-1.root-causes.json for a read case)
$ROOT/B/<case-id>-<ts>/run-1.*            eval/run.sh output of case B
$ROOT/barrier/A.json  B.json              exact product argv of each side, written on arrival
$ROOT/barrier/release.json                written once, only after both arrived
$ROOT/barrier/A.go.json  B.go.json        instant each side exec'd the real binary
$ROOT/barrier/wrapper-A  wrapper-B        what run.sh executed as $CODRAX_BIN
$ROOT/launch-intent-v1.json               argv, env, binary SHA-256, tool SHA-256 (before start)
$ROOT/launch-A-v1.log  launch-B-v1.log    stdout+stderr of each `bash eval/run.sh <case> 1`
$ROOT/launch-A-v1-receipt.json  …-B-…     formal exit code or signal, timestamps, run_dir, logger UTC offset
$ROOT/launch-receipt-v1.json              barrier_released, runs_per_case 1, product entries per side
```

* Exactly two `--case` values labelled `A` and `B`; `eval/run.sh` is always called with `1`.
* `--env KEY=VALUE` applies to both cases, `A:KEY=VALUE` / `B:KEY=VALUE` to one.
  `CODRAX_BIN`, `CODRAX_PROVIDER_ARGS_RAW` and `EVAL_RESULTS_ROOT` are set by the launcher.
* A root that already contains `A`, `B`, `barrier` or `launch-*` is refused: there is no
  second launch and no third run.
* If one side never reaches the product (prerequisite SKIP, a wrong case path), nothing is
  released: `barrier/aborted.json` is written, the other side exits 97 without starting a
  model process, and the launcher exits 1.
* `--timeout` is the outer wall-clock limit per case from spawn (default 1800 s): process
  group `SIGTERM`, `SIGKILL` 10 s later, `outer_timeout_hit: true`. Product waits
  (600/300/600 s) are not touched.
* `--bin` must be an absolute executable regular file. (`run.sh` silently rebuilds and runs
  `./codrax` when `CODRAX_BIN` is unusable; the launcher refuses instead.) With `--freeze`
  it must also be one of the manifest's external inputs with the same SHA-256, and the
  source inputs are re-verified before the start and after each case ends.
* The wrapper appends `--root-causes-out <result-dir>/run-1.root-causes.json` when the
  product argv has no `--mode` / `--write-*` argument, and changes nothing else.

### 5. Request overlap

```sh
python3 $T/overlap.py --repo $REPO --root $ROOT --out $W/actual-request-overlap-v1.json
# {"completed": {"A": 13, "B": 24}, "ambiguous": 0, "diagnostics": 0, "overlaps": 25, "max": 14.948, "actual_overlap_proven": true}
# 实际模型请求 A13/B24，25 对重叠、最长 14.948 秒，歧义 0
```

All parsing and interval arithmetic is `eval/request_intervals.py`; ambiguous groups are
reported and excluded. Manual form for logs that were not launched by `launch.py`:

```sh
python3 $T/overlap.py --repo $REPO --utc-offset-seconds -28800 \
  --a-log <A log> --a-started-at <ISO> --a-finished-at <ISO> \
  --b-log <B log> --b-started-at <ISO> --b-finished-at <ISO>
```

### 6. Human audits

Not a tool. Write `$W/A-audit-v1.md`, `$W/B-audit-v1.md` and, if used, an acceptance JSON.
No tool here reads or grades an answer.

### 7. Seal

```sh
python3 $T/seal.py seal --root $ROOT --tree $ROOT --tree $W --exclude $W/codrax-cli-v1 \
  --freeze $W/frozen-v1.json --prior $PREV \
  --attach full=$W/full-v1-stats.json --attach overlap=$W/actual-request-overlap-v1.json
# {"count": N, "index_sha256": "…", "receipt_sha256": "…", "prior_verified": 261}
# 封存 N 件，索引 SHA256 …，收据 …；前代 261 件逐份复验 261 件通过
```

Writes `$ROOT/sealed-v1/NNNN-<name>` (read-only byte copies; `.go` becomes `.go.txt`),
`$ROOT/sealed-index-v1.json` (`source` = canonical realpath, `archived`, `sha256`, `bytes`)
and `$ROOT/sealed-receipt-v1.json`. `--prior` re-hashes every piece of the previous batch
first and refuses to seal when it does not verify (`--allow-prior-mismatch` records the
failure instead). Product dumps outside the tree (`$REPO/.codrax/output/<ts>-<pid>.*`,
`.codrax/blob/<session>/…`) are added with `--file` or `--list`. A second generation after a
failed first one: `--generation v2`; the first is never rewritten.

`seal.py verify` reads the `sealed-index-vN.json` layout (83 indexes in 81 batch roots dated
2026-10-26 to 2026-11-11). The nine older roots that use `archive-vN.json` /
`artifact-sha256-vN.json` are not understood by it.

## What was checked without a model (2026-11-17, HEAD `1aceb24f3`)

| Check | Result |
|---|---|
| `suite_stats.py read` on the sealed §375 full log | every field equal to the sealed `full-stats-v1.json` (97/12/1, 28,819/34,731, 5 failed keys, SHA `0762b6e3…`) |
| `suite_stats.py run` on `./internal/canonpath ./internal/analysis/axis ./internal/analysis/priority` | 2 pass / 1 no-test, 8 top / 40 sub; `go test -v` shows the same 8 and 40 |
| `overlap.py` on the sealed §375 A and B logs | A13/B24, 25 pairs, 14.948 s, 0 ambiguous; interval endpoints equal to the sealed evidence |
| `seal.py verify` on §375 | 261 of 261, index `c32dfe4a…7e80`, receipt `f4d53bee…1a64` |
| `seal.py verify` on all 83 `sealed-index-vN.json` files (81 roots) | 33,029 of 33,029 pieces, every receipt count and index hash consistent |
| `freeze.py` on HEAD | 7,704 inputs; the 7,701 of the sealed §375 manifest are byte-identical; base `055ef5abd` gives the same 12 changed files and the same 12 snapshot digests as §375 |
| `launch.py` with a stub binary and the real `eval/run.sh` (harness copy, real case files) | both sides arrived, one release, product processes started 1.4 / 0.0 / 0.2 ms apart in three launches; A got `--root-causes-out`, B did not; argv shape equal to the §375 barrier records; second launch refused; one-sided arrival started nothing; outer timeout left no process |
| `launch.py` + `overlap.py --root` with a log-writing stub | A2/B2, 2 overlapping pairs, 0.705 s |
| `hmc_tools_test.py` | 12 tests OK |
