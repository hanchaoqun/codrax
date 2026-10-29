#!/usr/bin/env python3
"""Conservative request/response evidence from logger bytes, never a run verdict.

The current logger has no request ID. Once requests overlap, no member of that
busy group is uniquely paired, even when only one response remains outstanding.
Only a proven return to zero permits a later request to start a clean group.
"""
import argparse
from collections import Counter
from datetime import datetime, timedelta, timezone
import hashlib
import json
from pathlib import Path
import re


EVENT = re.compile(rb"^(\S+) (DEBUG|WARN) \[llm\] (request|response):(?:[ \t]|$)(.*)$")
MODEL = re.compile(rb"(?:^|[ \t])model=([^ \t\r\n]+)(?=[ \t\r\n]|$)")
EPOCH = datetime(1970, 1, 1, tzinfo=timezone.utc)


def instant(value, utc_offset_seconds=None):
    parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        if utc_offset_seconds is None:
            raise ValueError("naive timestamp requires explicit logger UTC offset")
        if type(utc_offset_seconds) is not int or abs(utc_offset_seconds) >= 86400:
            raise ValueError("invalid UTC offset")
        parsed = parsed.replace(tzinfo=timezone(timedelta(seconds=utc_offset_seconds)))
    utc = parsed.astimezone(timezone.utc)
    delta = utc - EPOCH
    micros = ((delta.days * 86400 + delta.seconds) * 1000000 + delta.microseconds)
    return micros, utc.isoformat(timespec="microseconds").replace("+00:00", "Z"), int(parsed.utcoffset().total_seconds())


def parse_log(raw, started_at, finished_at, *, log="<bytes>", utc_offset_seconds=None):
    """Pure bytes-to-evidence parser. Caller supplies process and clock provenance.

    Missing/malformed metadata never consumes another request by adjacency.
    Unknown-model responses cannot prove which outstanding bucket was closed.
    Unclosed log groups do not imply that the owning CLI process is still alive.
    """
    if not isinstance(raw, bytes):
        raise TypeError("raw log must be bytes")
    start_us = instant(started_at)[0]
    end_us = instant(finished_at)[0]
    if end_us < start_us:
        raise ValueError("process interval is reversed")
    digest = hashlib.sha256(raw).hexdigest()
    try:
        raw.decode("utf-8")
        valid_utf8 = True
    except UnicodeDecodeError:
        valid_utf8 = False
    complete, groups, diagnostics = [], [], []
    counts = {"requests": 0, "responses": 0, "recognized_events": 0}
    pending = Counter()
    active = None
    last_us = None

    for line_number, line in enumerate(raw.split(b"\n"), 1):
        match = EVENT.match(line)
        if not match:
            continue
        stamp, level, kind, tail = match.groups()
        kind = kind.decode("ascii")
        counts["recognized_events"] += 1
        counts["requests" if kind == "request" else "responses"] += 1
        row = {"log": log, "line": line_number, "kind": kind, "level": level.decode("ascii")}
        errors = []
        models = MODEL.findall(tail)
        model = None
        if len(models) == 1:
            try:
                model = models[0].decode("utf-8")
                if not model or any(ord(c) < 32 or ord(c) == 127 for c in model):
                    raise ValueError("invalid model")
            except (UnicodeDecodeError, ValueError):
                errors.append("invalid_model_metadata")
                model = None
        else:
            errors.append("missing_or_duplicate_model_metadata")
        row["model"] = model
        try:
            row["timestamp"] = stamp.decode("ascii")
            at_us, at_utc, offset = instant(row["timestamp"], utc_offset_seconds)
            row.update(epoch_us=at_us, epoch=at_us / 1000000,
                       at_utc=at_utc, utc_offset_seconds=offset)
            if at_us < start_us or at_us > end_us:
                errors.append("outside_formal_process_lifetime")
            if last_us is not None and at_us < last_us:
                errors.append("nonmonotonic_event_timestamp")
            last_us = at_us if last_us is None else max(last_us, at_us)
        except (ValueError, UnicodeDecodeError, OverflowError):
            errors.append("invalid_or_unresolved_timestamp")
        row["errors"] = errors
        if errors:
            diagnostics.append({"event": row, "reason": "invalid_event_metadata"})

        if kind == "request":
            if active is None:
                active = {"log": log, "requests": [], "responses": [], "ambiguous": False,
                          "invalid": False, "first_line": line_number}
            elif sum(pending.values()) > 0:
                active["ambiguous"] = True
            active["requests"].append(row)
            active["invalid"] = active["invalid"] or bool(errors)
            # None deliberately cannot be discharged by a later response.
            pending[model] += 1
            continue

        if active is None:
            diagnostics.append({"event": row, "reason": "response_without_pending_request"})
            continue
        active["responses"].append(row)
        active["invalid"] = active["invalid"] or bool(errors)
        if errors:
            # An unusable clock/model cannot certify this response belongs to
            # the outstanding process-scoped group, including its last member.
            continue
        if model is None or pending[model] <= 0:
            active["invalid"] = True
            diagnostics.append({"event": row, "reason": "response_model_not_identifiable_in_pending_group"})
            continue
        # This proves only the group/model cardinality, never a member pairing.
        pending[model] -= 1
        if sum(pending.values()) != 0:
            continue
        active.update(closed=True, last_line=line_number, outstanding_requests=0,
                      outstanding_by_model=[])
        if not active["ambiguous"] and not active["invalid"] and len(active["requests"]) == 1:
            first = active["requests"][0]
            duration_us = row["epoch_us"] - first["epoch_us"]
            complete.append({"start": first, "finish": row,
                             "duration_microseconds": duration_us,
                             "duration_seconds": duration_us / 1000000,
                             "log_sha256": digest})
        else:
            active["reason"] = "ambiguous_or_invalid_busy_group"
            groups.append(active)
        active = None
        pending.clear()

    if active is not None:
        active.update(closed=False, outstanding_requests=sum(pending.values()),
                      outstanding_by_model=[{"model": model, "count": count}
                                            for model, count in pending.items() if count],
                      reason="unfinished_log_group_not_process_state")
        groups.append(active)
    return {"schema": "codrax.request_intervals.v1", "completed": complete,
            "ambiguous": groups, "diagnostics": diagnostics, "counts": counts,
            "unresolved_request_count": sum(pending.values()), "log_sha256": digest,
            "valid_utf8": valid_utf8, "logger_utc_offset_seconds": utc_offset_seconds,
            "scope": "Only uniquely proven logger intervals; no case verdict, process state, or remote request identity inferred"}


def overlaps(left, right):
    """Positive intersections of already-proven intervals, with integer precision."""
    result = []
    for a in left:
        for b in right:
            lo = max(a["start"]["epoch_us"], b["start"]["epoch_us"])
            hi = min(a["finish"]["epoch_us"], b["finish"]["epoch_us"])
            if hi > lo:
                result.append({"a_start": a["start"], "a_finish": a["finish"],
                               "b_start": b["start"], "b_finish": b["finish"],
                               "microseconds": hi - lo, "seconds": (hi - lo) / 1000000,
                               "start_utc": (EPOCH + timedelta(microseconds=lo)).isoformat(timespec="microseconds").replace("+00:00", "Z"),
                               "end_utc": (EPOCH + timedelta(microseconds=hi)).isoformat(timespec="microseconds").replace("+00:00", "Z")})
    return result


def read_nonsecret(path):
    path = Path(path)
    actual = path.resolve(strict=True)
    for candidate in (path, actual):
        if any(re.match(r"^(providers(?:\.|$)|\.env(?:\.|$))", part, re.I) for part in candidate.parts):
            raise ValueError("credential path forbidden")
    if path.is_symlink() or not path.is_file():
        raise ValueError("regular nonsymlink file required")
    return actual, path.read_bytes()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", required=True)
    parser.add_argument("--started-at", required=True)
    parser.add_argument("--finished-at", required=True)
    parser.add_argument("--utc-offset-seconds", type=int)
    args = parser.parse_args()
    path, raw = read_nonsecret(args.log)
    result = parse_log(raw, args.started_at, args.finished_at,
                       log=str(path), utc_offset_seconds=args.utc_offset_seconds)
    print(json.dumps(result, ensure_ascii=True, indent=2))


if __name__ == "__main__":
    main()
