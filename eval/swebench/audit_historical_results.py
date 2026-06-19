#!/usr/bin/env python3
"""Audit historical Codrax SWE-bench results against their emitted patches.

This is an offline audit helper, not a scorer. The official SWE-bench harness is
the only authoritative resolved/not-resolved metric. This helper consumes local
Codrax result artifacts plus, when available, the public SWE-bench dataset's
gold patch for post-hoc comparison. Gold patches are never needed to run Codrax;
they are used here only to make the manual review queue reproducible.
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from collections import Counter
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import summarize_codrax_results as codrax_results  # noqa: E402


SOURCE_EXTENSIONS = {
    ".c",
    ".cc",
    ".cpp",
    ".cs",
    ".go",
    ".h",
    ".hpp",
    ".java",
    ".js",
    ".jsx",
    ".kt",
    ".m",
    ".mm",
    ".php",
    ".py",
    ".rb",
    ".rs",
    ".scala",
    ".sh",
    ".swift",
    ".ts",
    ".tsx",
    ".vue",
}

DOC_EXTENSIONS = {".md", ".rst", ".txt"}

TEST_DIR_NAMES = {
    "__tests__",
    "spec",
    "specs",
    "test",
    "tests",
    "testing",
}


@dataclass(frozen=True)
class DiffSummary:
    files: list[str]
    source_files: list[str]
    test_files: list[str]
    doc_files: list[str]
    tokens: set[str]
    line_count: int


def text(row: dict[str, Any], key: str) -> str:
    return str(row.get(key) or "").strip()


def int_value(row: dict[str, Any], key: str) -> int:
    return codrax_results.int_value(row, key)


def load_json_object(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError:
        return {}
    return data if isinstance(data, dict) else {}


def load_prediction_patch(instance_dir: Path) -> str:
    prediction = load_json_object(instance_dir / "prediction.json")
    patch = prediction.get("model_patch")
    return patch if isinstance(patch, str) else ""


def load_instance(instance_dir: Path) -> dict[str, Any]:
    return load_json_object(instance_dir / "instance.json")


def resolve_final_report_path(row: dict[str, Any], instance_dir: Path) -> Path | None:
    explicit = text(row, "final_report_path")
    if explicit:
        path = Path(explicit)
        if path.exists():
            return path
    report_path = text(row, "report_path")
    if report_path:
        path = Path(report_path).with_name(Path(report_path).stem.replace(".report", "") + ".final.json")
        if path.exists():
            return path
    prediction = instance_dir / "prediction.json"
    if prediction.exists():
        candidates = sorted(instance_dir.glob("repo/.codrax/plans/*.final.json"))
        if candidates:
            return candidates[-1]
    return None


def tail_text(path: Path, max_lines: int = 16, max_chars: int = 4000) -> str:
    if not path.exists():
        return ""
    try:
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return ""
    tail = "\n".join(lines[-max_lines:]).strip()
    if len(tail) <= max_chars:
        return tail
    return tail[-max_chars:]


def result_instance_dir(row: dict[str, Any]) -> Path:
    value = text(row, "instance_dir")
    if value:
        return Path(value)
    source_path = Path(text(row, "__source_path"))
    return source_path.parent / "instances" / text(row, "instance_id")


def parse_diff_files(patch: str) -> list[str]:
    out: list[str] = []
    current: str | None = None
    for line in patch.splitlines():
        if line.startswith("diff --git "):
            current = None
            parts = line.split()
            if len(parts) >= 4:
                current = strip_diff_prefix(parts[3])
                if current and current != "/dev/null":
                    out.append(current)
            continue
        if line.startswith("+++ "):
            path = line[4:].strip()
            if path == "/dev/null":
                continue
            path = strip_diff_prefix(path)
            if path and path not in out:
                out.append(path)
    return out


def strip_diff_prefix(path: str) -> str:
    if path.startswith('"') and path.endswith('"'):
        path = path[1:-1]
    for prefix in ("a/", "b/"):
        if path.startswith(prefix):
            return path[len(prefix) :]
    return path


def is_test_path(path: str) -> bool:
    parts = [part.lower() for part in Path(path).parts]
    name = parts[-1] if parts else path.lower()
    stem = Path(name).stem.lower()
    suffix = Path(name).suffix.lower()
    if any(part in TEST_DIR_NAMES for part in parts):
        return True
    if name.startswith("test_") or name.endswith("_test.py") or name.endswith(".test.js"):
        return True
    if name.endswith(".spec.js") or name.endswith(".spec.ts") or name.endswith(".test.ts"):
        return True
    return suffix == ".t" or stem.endswith("_spec")


def is_doc_path(path: str) -> bool:
    suffix = Path(path).suffix.lower()
    lower = path.lower()
    return suffix in DOC_EXTENSIONS or lower.startswith("docs/")


def is_source_path(path: str) -> bool:
    suffix = Path(path).suffix.lower()
    if suffix not in SOURCE_EXTENSIONS:
        return False
    return not is_test_path(path)


TOKEN_RE = re.compile(r"[A-Za-z_][A-Za-z0-9_]*|[0-9]+|==|!=|<=|>=|&&|\|\||::|->")


def changed_diff_lines(patch: str) -> list[str]:
    lines: list[str] = []
    for raw in patch.splitlines():
        if not raw or raw[:3] in {"+++", "---"}:
            continue
        if raw[0] not in {"+", "-"}:
            continue
        content = raw[1:].strip()
        if content:
            lines.append(content)
    return lines


def patch_tokens(patch: str) -> set[str]:
    tokens: set[str] = set()
    for line in changed_diff_lines(patch):
        tokens.update(match.group(0).lower() for match in TOKEN_RE.finditer(line))
    return tokens


def diff_summary(patch: str) -> DiffSummary:
    files = parse_diff_files(patch)
    return DiffSummary(
        files=files,
        source_files=[path for path in files if is_source_path(path)],
        test_files=[path for path in files if is_test_path(path)],
        doc_files=[path for path in files if is_doc_path(path)],
        tokens=patch_tokens(patch),
        line_count=len(changed_diff_lines(patch)),
    )


def ratio(num: int, den: int) -> float:
    if den <= 0:
        return 0.0
    return num / den


def jaccard(left: Iterable[str], right: Iterable[str]) -> float:
    left_set = set(left)
    right_set = set(right)
    if not left_set and not right_set:
        return 1.0
    union = left_set | right_set
    if not union:
        return 0.0
    return len(left_set & right_set) / len(union)


def classify_theoretical_fit(
    row: dict[str, Any],
    model: DiffSummary,
    oracle: DiffSummary | None,
) -> tuple[str, str]:
    """Return conservative audit verdict and typed reason code.

    The logic intentionally uses artifact structure: patch presence, changed
    file classes, oracle source-file overlap, token overlap, and typed verifier
    status. It does not read issue wording or model prose for routing.
    """

    if not model.files or int_value(row, "patch_bytes") <= 0:
        return "fail", "empty_patch"
    if model.files and not model.source_files:
        return "fail", "no_source_change"
    if oracle is None:
        return "unknown", "oracle_patch_unavailable"
    if not oracle.source_files:
        return "unknown", "oracle_has_no_source_change"

    model_source = set(model.source_files)
    oracle_source = set(oracle.source_files)
    source_overlap = model_source & oracle_source
    if not source_overlap:
        return "fail", "wrong_source_surface_no_oracle_overlap"

    file_overlap_ratio = ratio(len(source_overlap), len(oracle_source))
    token_overlap = jaccard(model.tokens, oracle.tokens)
    verify_status = text(row, "verify_status")
    verify_kind = text(row, "verify_failure_kind")
    local_acceptance = text(row, "local_acceptance_verdict")
    local_acceptance_source = text(row, "local_acceptance_source")

    if verify_status == "failed" and verify_kind in {"tests_failed", "build_failure", "timeout"}:
        return "fail", "typed_verify_failed"
    if local_acceptance == "pass" and local_acceptance_source == "local_verify":
        return "pass", "local_verify_passed"
    if verify_status == "passed" and token_overlap >= 0.20:
        return "pass", "verify_passed_with_oracle_overlap"
    if file_overlap_ratio >= 0.75 and token_overlap >= 0.28:
        return "pass", "oracle_source_and_token_overlap_high"
    if file_overlap_ratio >= 0.50 and token_overlap >= 0.18 and verify_status != "failed":
        return "unknown", "plausible_overlap_unverified"
    if token_overlap < 0.08:
        return "fail", "weak_semantic_overlap"
    return "unknown", "source_overlap_requires_manual_review"


def first_problem_line(instance: dict[str, Any]) -> str:
    problem = str(instance.get("problem_statement") or "").strip()
    if not problem:
        return ""
    for line in problem.splitlines():
        line = line.strip()
        if line:
            return line[:160]
    return ""


def load_oracle_rows(dataset_name: str, split: str) -> dict[str, dict[str, Any]]:
    try:
        from datasets import load_dataset  # type: ignore
    except ImportError as exc:
        raise SystemExit(
            "the datasets package is required for --dataset-name oracle audit; "
            "install it in eval/results/swebench/.venv or omit --dataset-name"
        ) from exc
    dataset = load_dataset(dataset_name, split=split)
    rows: dict[str, dict[str, Any]] = {}
    for item in dataset:
        if not isinstance(item, dict):
            continue
        instance_id = str(item.get("instance_id") or "").strip()
        if instance_id:
            rows[instance_id] = item
    return rows


def audit_rows(rows: list[dict[str, Any]], oracle_rows: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    for row in rows:
        instance_id = text(row, "instance_id")
        instance_dir = result_instance_dir(row)
        instance = load_instance(instance_dir)
        codrax_out_path = instance_dir / "codrax.out"
        final_report_path = resolve_final_report_path(row, instance_dir)
        final_report = load_json_object(final_report_path) if final_report_path else {}
        reasoning_graph = final_report.get("reasoning_graph") if isinstance(final_report.get("reasoning_graph"), dict) else {}
        proof_ledger = final_report.get("proof_ledger") if isinstance(final_report.get("proof_ledger"), dict) else {}
        model_patch = load_prediction_patch(instance_dir)
        model_summary = diff_summary(model_patch)
        oracle_row = oracle_rows.get(instance_id, {})
        oracle_patch = oracle_row.get("patch")
        oracle_summary = diff_summary(oracle_patch) if isinstance(oracle_patch, str) else None
        verdict, reason = classify_theoretical_fit(row, model_summary, oracle_summary)
        oracle_files = oracle_summary.source_files if oracle_summary else []
        source_overlap = sorted(set(model_summary.source_files) & set(oracle_files))
        oracle_problem = str(oracle_row.get("problem_statement") or "").splitlines()
        item = {
            "instance_id": instance_id,
            "repo": text(row, "repo") or str(instance.get("repo") or oracle_row.get("repo") or ""),
            "problem": first_problem_line(instance) or (oracle_problem[0][:160] if oracle_problem else ""),
            "audit_verdict": verdict,
            "audit_reason_code": reason,
            "patch_bytes": int_value(row, "patch_bytes"),
            "model_files": model_summary.files,
            "model_source_files": model_summary.source_files,
            "model_test_files": model_summary.test_files,
            "oracle_source_files": oracle_files,
            "source_overlap_files": source_overlap,
            "source_overlap_ratio": round(ratio(len(source_overlap), len(oracle_files)), 4),
            "token_jaccard": round(jaccard(model_summary.tokens, oracle_summary.tokens if oracle_summary else set()), 4),
            "verify_status": text(row, "verify_status"),
            "verify_failure_kind": text(row, "verify_failure_kind"),
            "verify_failure_reason_code": text(row, "verify_failure_reason_code"),
            "prediction_verdict": text(row, "prediction_verdict"),
            "prediction_local_confidence": text(row, "prediction_local_confidence"),
            "prediction_confidence_downgrade_reason": text(row, "prediction_confidence_downgrade_reason"),
            "prediction_audit_block_reason": text(row, "prediction_audit_block_reason"),
            "workflow_status": text(row, "workflow_status"),
            "workflow_latest_progress_stage": text(row, "workflow_latest_progress_stage"),
            "workflow_latest_progress_reason_code": text(row, "workflow_latest_progress_reason_code"),
            "local_acceptance_verdict": text(row, "local_acceptance_verdict"),
            "local_acceptance_source": text(row, "local_acceptance_source"),
            "manual_audit_verdict": text(row, "manual_audit_verdict"),
            "manual_audit_reason_code": text(row, "manual_audit_reason_code"),
            "manual_audit_functional_verdict": text(row, "manual_audit_functional_verdict"),
            "manual_audit_functional_reason_code": text(row, "manual_audit_functional_reason_code"),
            "manual_audit_fidelity_verdict": text(row, "manual_audit_fidelity_verdict"),
            "manual_audit_fidelity_reason_code": text(row, "manual_audit_fidelity_reason_code"),
            "dropped_test_patch_paths": row.get("dropped_test_patch_paths") if isinstance(row.get("dropped_test_patch_paths"), list) else [],
            "final_answer_audit_surface": "write_final_report" if final_report else ("codrax_out_tail" if codrax_out_path.exists() else "missing"),
            "final_answer_typed_artifact_present": bool(final_report),
            "final_report_path": str(final_report_path) if final_report_path else text(row, "final_report_path"),
            "final_report_completion_verdict": str(((final_report.get("completion") if isinstance(final_report.get("completion"), dict) else {}) or {}).get("verdict") or ""),
            "final_report_residual_risk_codes": [
                str(risk.get("code") or "").strip()
                for risk in (final_report.get("residual_risks") or [])
                if isinstance(risk, dict) and str(risk.get("code") or "").strip()
            ],
            "final_report_proof_ledger_uncovered_count": int(
                proof_ledger.get("uncovered_count") or int_value(row, "final_report_proof_ledger_uncovered_count")
            ),
            "final_report_proof_ledger_unavailable_count": int(
                proof_ledger.get("unavailable_count") or int_value(row, "final_report_proof_ledger_unavailable_count")
            ),
            "final_report_proof_ledger_reason_codes": [
                str(value).strip()
                for value in (proof_ledger.get("reason_codes") or row.get("final_report_proof_ledger_reason_codes") or [])
                if str(value).strip()
            ],
            "final_report_proof_ledger_obligation_reason_codes": [
                str(value).strip()
                for value in (
                    proof_ledger.get("obligation_reason_codes")
                    or row.get("final_report_proof_ledger_obligation_reason_codes")
                    or []
                )
                if str(value).strip()
            ],
            "final_report_reasoning_graph_event_count": int(reasoning_graph.get("event_count") or int_value(row, "final_report_reasoning_graph_event_count")),
            "final_report_reasoning_graph_repair_event_count": int(reasoning_graph.get("repair_event_count") or int_value(row, "final_report_reasoning_graph_repair_event_count")),
            "final_report_reasoning_graph_llm_event_count": int(reasoning_graph.get("llm_event_count") or int_value(row, "final_report_reasoning_graph_llm_event_count")),
            "final_report_reasoning_graph_last_reason_code": str(
                reasoning_graph.get("last_reason_code") or text(row, "final_report_reasoning_graph_last_reason_code")
            ).strip(),
            "final_report_reasoning_graph_event_refs": [
                str(value).strip()
                for value in (reasoning_graph.get("event_refs") or row.get("final_report_reasoning_graph_event_refs") or [])
                if str(value).strip()
            ],
            "codrax_out_bytes": codrax_out_path.stat().st_size if codrax_out_path.exists() else 0,
            "codrax_out_tail": tail_text(codrax_out_path),
            "instance_dir": str(instance_dir),
            "report_path": text(row, "report_path"),
            "source_results_jsonl": text(row, "__source_path"),
            "source_results_line": int_value(row, "__source_line"),
        }
        item["graph_present"] = codrax_results.row_graph_present(item)
        item["graph_event_count"] = codrax_results.row_graph_event_count(item)
        item["graph_repair_event_count"] = codrax_results.row_graph_repair_event_count(item)
        item["graph_missing_p0_evidence_count"] = codrax_results.row_graph_missing_p0_evidence_count(item)
        item["graph_unverified_reason_codes"] = codrax_results.row_graph_unverified_reason_codes(item)
        out.append(item)
    return out


def counts(rows: list[dict[str, Any]], key: str) -> dict[str, int]:
    return dict(sorted(Counter(str(row.get(key) or "<empty>") for row in rows).items()))


def top_reason_rows(rows: list[dict[str, Any]], limit: int = 8) -> list[tuple[str, int]]:
    return Counter(str(row.get("audit_reason_code") or "<empty>") for row in rows).most_common(limit)


def pct(part: int, total: int) -> str:
    if total <= 0:
        return "n/a"
    return f"{part / total * 100:.1f}%"


def md_escape(value: Any) -> str:
    text_value = str(value or "")
    return text_value.replace("|", "\\|").replace("\n", " ")


def compact_files(files: list[str], limit: int = 3) -> str:
    if len(files) <= limit:
        return ", ".join(files)
    return ", ".join(files[:limit]) + f", +{len(files) - limit}"


def render_markdown(rows: list[dict[str, Any]], summary: dict[str, Any], *, dataset_name: str, split: str) -> str:
    total = len(rows)
    verdict_counts = Counter(str(row.get("audit_verdict") or "<empty>") for row in rows)
    reason_counts = Counter(str(row.get("audit_reason_code") or "<empty>") for row in rows)
    generated = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M:%S UTC")
    lines: list[str] = []
    lines.append("# SWE-bench Historical Patch Audit")
    lines.append("")
    lines.append(f"Generated: {generated}")
    lines.append("")
    lines.append("Scope: latest historical Codrax result per SWE-bench instance under `eval/results/swebench`.")
    lines.append("")
    lines.append(
        "Important: this is an oracle-assisted post-hoc audit, not an official SWE-bench score. "
        "The public dataset gold patch is used only to review whether Codrax touched the expected source surface and made a plausibly equivalent change."
    )
    lines.append("")
    lines.append("## Summary")
    lines.append("")
    lines.append(f"- Dataset: `{dataset_name}` split `{split}`.")
    lines.append(f"- Input result rows: {summary.get('input_row_count', total)}; unique latest instances: {total}.")
    for verdict in ("pass", "fail", "unknown"):
        count = verdict_counts.get(verdict, 0)
        lines.append(f"- Theoretical audit `{verdict}`: {count}/{total} ({pct(count, total)}).")
    lines.append("- Official resolved rate: not computed here; run the official SWE-bench harness for authoritative scoring.")
    lines.append("")
    lines.append("## Reason Counts")
    lines.append("")
    lines.append("| reason | count |")
    lines.append("| --- | ---: |")
    for reason, count in reason_counts.most_common():
        lines.append(f"| `{md_escape(reason)}` | {count} |")
    lines.append("")
    lines.append("## System Shortfalls From The Audit")
    lines.append("")
    lines.append(
        "- Exploration/localization is still the largest product gap: many failed rows produce a non-empty patch, but the changed source files do not intersect the oracle source surface. "
        "That means the system often writes code before it has pinned the causal file/symbol."
    )
    lines.append(
        "- Verify evidence is frequently not decisive in historical artifacts: legacy rows lack the current local-acceptance fields, and several rows end in unverified/failed states while still exporting patches. "
        "The adapter now exposes this as an unevaluable denominator rather than a pass rate."
    )
    lines.append(
        "- Handoff quality needs to be measured by whether failure observations change the next patch surface, not only by whether context packs exist. "
        "Rows with source overlap but low token overlap should feed patch critic, impact analysis, and re-exploration."
    )
    lines.append(
        "- The write loop should prefer online edit-run-observe batches: small source-surface changes, typed observation, then continue. "
        "Large one-shot patches make wrong localization expensive and make verifier failures harder to attribute."
    )
    lines.append(
        "- Final answer quality cannot be audited from old result rows alone. Future SWE runs should persist a typed final-answer artifact with patch intent, touched contracts, observed failures, and residual risk."
    )
    final_report_count = sum(1 for row in rows if row.get("final_report_present") or row.get("final_report_path"))
    graph_present_count = sum(1 for row in rows if row.get("graph_present"))
    graph_missing_p0_total = sum(int(row.get("graph_missing_p0_evidence_count") or 0) for row in rows)
    codrax_out_count = sum(1 for row in rows if row.get("codrax_output_path") or row.get("codrax_out_path"))
    if final_report_count:
        lines.append(
            f"- Typed final-report artifacts are present for {final_report_count}/{total} audited instance(s). "
            "Prefer these structured artifacts over terminal logs for delivery audit and residual-risk analysis."
        )
    else:
        lines.append(
            "- No audited instance has a typed final-answer/report artifact. "
            "Production eval should not rely on free-form terminal logs as the answer contract."
        )
    if graph_present_count:
        lines.append(
            f"- Reasoning graph telemetry is present for {graph_present_count}/{total} audited instance(s); "
            f"typed P0 evidence gaps total {graph_missing_p0_total}."
        )
        reason_counts = Counter(
            reason
            for row in rows
            for reason in (row.get("graph_unverified_reason_codes") or [])
            if str(reason or "").strip()
        )
        if reason_counts:
            rendered = ", ".join(f"`{md_escape(reason)}`={count}" for reason, count in reason_counts.most_common(6))
            lines.append(f"- Top graph unverified reason codes: {rendered}.")
    if codrax_out_count:
        lines.append(
            f"- Free-form `codrax.out` logs are present for {codrax_out_count}/{total} audited instance(s); "
            "they remain human-debug context only and must not drive product pass/fail logic."
        )
    lines.append("")
    lines.append("## Per-Instance Audit")
    lines.append("")
    lines.append("| instance | repo | verdict | reason | model source | oracle source | overlap | token | verify | graph | graph reason |")
    lines.append("| --- | --- | --- | --- | --- | --- | ---: | ---: | --- | ---: | --- |")
    for row in sorted(rows, key=lambda item: str(item.get("instance_id") or "")):
        lines.append(
            "| "
            + " | ".join(
                [
                    f"`{md_escape(row.get('instance_id'))}`",
                    md_escape(row.get("repo")),
                    f"`{md_escape(row.get('audit_verdict'))}`",
                    f"`{md_escape(row.get('audit_reason_code'))}`",
                    md_escape(compact_files(row.get("model_source_files") or [])),
                    md_escape(compact_files(row.get("oracle_source_files") or [])),
                    str(row.get("source_overlap_ratio")),
                    str(row.get("token_jaccard")),
                    f"`{md_escape(row.get('verify_status'))}`",
                    str(row.get("graph_event_count") or 0),
                    f"`{md_escape(row.get('final_report_reasoning_graph_last_reason_code'))}`",
                ]
            )
            + " |"
        )
    lines.append("")
    lines.append("## Reproduction")
    lines.append("")
    lines.append("```bash")
    lines.append(
        "eval/results/swebench/.venv/bin/python eval/swebench/audit_historical_results.py "
        "--results-glob 'eval/results/swebench/*/results.jsonl' "
        "--dedupe latest-by-file-mtime "
        "--dataset-name SWE-bench/SWE-bench_Lite --split test "
        "--output-jsonl docs/design/swebench_historical_patch_audit_20260618.jsonl "
        "--output-md docs/design/swebench_historical_patch_audit_20260618.md"
    )
    lines.append("```")
    lines.append("")
    return "\n".join(lines)


def write_jsonl(path: Path, rows: list[dict[str, Any]]) -> None:
    with path.open("w", encoding="utf-8") as handle:
        for row in rows:
            handle.write(json.dumps(row, sort_keys=True, ensure_ascii=False) + "\n")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--results-jsonl", type=Path, action="append", default=[])
    parser.add_argument("--results-glob", action="append", default=[])
    parser.add_argument(
        "--dedupe",
        choices=["none", "latest-by-file-mtime"],
        default="latest-by-file-mtime",
    )
    parser.add_argument("--dataset-name", default="SWE-bench/SWE-bench_Lite")
    parser.add_argument("--split", default="test")
    parser.add_argument("--output-jsonl", type=Path)
    parser.add_argument("--output-md", type=Path)
    parser.add_argument("--json-only", action="store_true")
    args = parser.parse_args()

    paths = codrax_results.expand_results_paths(args.results_jsonl, args.results_glob)
    if not paths:
        raise SystemExit("at least one --results-jsonl or --results-glob is required")
    input_rows = codrax_results.load_results_files(paths)
    rows = input_rows
    if args.dedupe == "latest-by-file-mtime":
        rows = codrax_results.dedupe_latest_by_instance(input_rows)

    oracle_rows = load_oracle_rows(args.dataset_name, args.split)
    audit = audit_rows(rows, oracle_rows)
    summary = codrax_results.summarize_results(
        rows,
        input_row_count=len(input_rows),
        input_paths=[str(path) for path in paths],
        dedupe_mode=args.dedupe,
    )

    if args.output_jsonl:
        args.output_jsonl.parent.mkdir(parents=True, exist_ok=True)
        write_jsonl(args.output_jsonl, audit)
    if args.output_md:
        args.output_md.parent.mkdir(parents=True, exist_ok=True)
        args.output_md.write_text(
            render_markdown(audit, summary, dataset_name=args.dataset_name, split=args.split),
            encoding="utf-8",
        )

    encoded = json.dumps(
        {
            "row_count": len(audit),
            "verdict_counts": counts(audit, "audit_verdict"),
            "reason_counts": counts(audit, "audit_reason_code"),
            "top_reasons": top_reason_rows(audit),
            "output_jsonl": str(args.output_jsonl) if args.output_jsonl else "",
            "output_md": str(args.output_md) if args.output_md else "",
        },
        indent=2,
        sort_keys=True,
        ensure_ascii=False,
    )
    if args.json_only:
        print(encoded)
    else:
        print(encoded)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
