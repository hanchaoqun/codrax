#!/usr/bin/env python3
"""Tiny stdio MCP server used by Codrax evals.

The fixture intentionally exercises the real JSON-RPC MCP path instead of a
mocked in-process server: initialize, tools/list, tools/call, resources/list,
resources/read, and prompts/list all travel over stdin/stdout.
"""

from __future__ import annotations

import json
import sys
from typing import Any


TRACE_RESOURCE_URI = "mcp://fixture/trace/sleep-wakeup"
OBS_MIME = "application/vnd.codrax.observation+json"

TRACE_LINES = [
    "# fixture ftrace-style runtime artifact",
    "# TASK-PID TGID CPU# TIMESTAMP FUNCTION",
    "      worker-100 (100) [000] .... 10.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=100 next_prio=20",
    "      worker-100 (100) [000] .... 10.001000: print: B|100|fixture-start",
    "      worker-100 (100) [000] .... 10.002000: sched_wakeup: comm=target pid=4242 prio=30 target_cpu=000",
    "      target-4242 (4242) [000] .... 10.003000: sched_switch: prev_comm=worker prev_pid=100 prev_prio=20 prev_state=R+ ==> next_comm=target next_pid=4242 next_prio=30",
    "      target-4242 (4242) [000] .... 10.010000: sched_switch: prev_comm=target prev_pid=4242 prev_prio=30 prev_state=S ==> next_comm=worker next_pid=100 next_prio=20",
    "      worker-100 (100) [000] .... 10.020000: cpu_frequency: state=1600000 cpu_id=0",
    "      worker-100 (100) [000] .... 10.050000: block_rq_issue: 12,48 RS 4096 () 1000 + 8 [worker]",
    "      irq-73 (2) [000] .... 10.060000: block_rq_complete: 12,48 RS () 1000 + 8 [0]",
    "      worker-100 (100) [000] .... 10.100000: sched_wakeup: comm=helper pid=3131 prio=25 target_cpu=000",
    "      helper-3131 (3131) [000] .... 10.145000: sched_wakeup: comm=target pid=4242 prio=30 target_cpu=000",
    "      target-4242 (4242) [000] .... 10.146000: sched_switch: prev_comm=helper prev_pid=3131 prev_prio=25 prev_state=R+ ==> next_comm=target next_pid=4242 next_prio=30",
    "      target-4242 (4242) [000] .... 10.147000: print: E|4242",
]


def observation_envelope() -> dict[str, Any]:
    return {
        "version": "codrax.mcp.observation.v1",
        "summary": "fixture MCP typed line observations for target sleep/wakeup",
        "resource_uri": TRACE_RESOURCE_URI,
        "mime_type": "text/plain",
        "observations": [
            {
                "summary": "target enters S sleep",
                "resource_uri": TRACE_RESOURCE_URI,
                "line_start": 7,
                "line_end": 7,
                "selector": "pid=4242 event=sched_switch prev_state=S",
                "raw_ref": f"{TRACE_RESOURCE_URI}#L7",
            },
            {
                "summary": "helper wakes target",
                "resource_uri": TRACE_RESOURCE_URI,
                "line_start": 12,
                "line_end": 12,
                "selector": "pid=4242 event=sched_wakeup waker=helper",
                "raw_ref": f"{TRACE_RESOURCE_URI}#L12",
            },
        ],
    }


def tool_schema() -> dict[str, Any]:
    return {
        "name": "lookup_trace_fact",
        "description": (
            "Return deterministic typed line observations for the fixture trace. "
            "Use this when the question asks about MCP fixture sleep/wakeup facts."
        ),
        "inputSchema": {
            "type": "object",
            "properties": {
                "query": {
                    "type": "string",
                    "description": "Natural-language lookup query.",
                }
            },
            "additionalProperties": False,
        },
    }


def handle(method: str, params: Any) -> Any:
    if method == "initialize":
        return {
            "protocolVersion": "2025-03-26",
            "capabilities": {
                "tools": {},
                "resources": {},
                "prompts": {},
            },
            "serverInfo": {"name": "codrax-eval-typed-line-fixture", "version": "1.0.0"},
        }
    if method == "tools/list":
        return {"tools": [tool_schema()]}
    if method == "tools/call":
        name = (params or {}).get("name")
        if name != "lookup_trace_fact":
            return {
                "isError": True,
                "content": [{"type": "text", "text": f"unknown fixture tool: {name}"}],
            }
        return {
            "content": [
                {
                    "type": "text",
                    "mimeType": OBS_MIME,
                    "text": json.dumps(observation_envelope(), ensure_ascii=False),
                }
            ]
        }
    if method == "resources/list":
        return {
            "resources": [
                {
                    "uri": TRACE_RESOURCE_URI,
                    "name": "fixture trace with typed line facts",
                    "description": "Small ftrace-like text artifact used to verify MCP typed line support.",
                    "mimeType": "text/plain",
                }
            ]
        }
    if method == "resources/read":
        uri = (params or {}).get("uri")
        if uri != TRACE_RESOURCE_URI:
            raise ValueError(f"resource not found: {uri}")
        return {
            "contents": [
                {
                    "uri": TRACE_RESOURCE_URI,
                    "mimeType": "text/plain",
                    "text": "\n".join(TRACE_LINES) + "\n",
                },
                {
                    "uri": TRACE_RESOURCE_URI,
                    "mimeType": OBS_MIME,
                    "text": json.dumps(observation_envelope(), ensure_ascii=False),
                },
            ]
        }
    if method == "prompts/list":
        return {
            "prompts": [
                {
                    "name": "trace_sleep_lookup",
                    "description": "External suggestion only: ask lookup_trace_fact for typed line sleep/wakeup facts.",
                }
            ]
        }
    if method == "notifications/initialized":
        return None
    raise ValueError(f"unsupported method: {method}")


def respond(msg: dict[str, Any], result: Any = None, error: Exception | None = None) -> None:
    if "id" not in msg:
        return
    out: dict[str, Any] = {"jsonrpc": "2.0", "id": msg["id"]}
    if error is not None:
        out["error"] = {"code": -32000, "message": str(error)}
    else:
        out["result"] = result if result is not None else {}
    sys.stdout.write(json.dumps(out, ensure_ascii=False) + "\n")
    sys.stdout.flush()


def main() -> int:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
            method = msg.get("method", "")
            result = handle(method, msg.get("params"))
            respond(msg, result=result)
        except Exception as exc:  # JSON-RPC fixture errors should be explicit.
            respond(msg if "msg" in locals() and isinstance(msg, dict) else {}, error=exc)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
