"""Async pipeline entry point."""

import asyncio

from . import plugins  # noqa: F401 — import populates REGISTRY
from .registry import resolve


async def run_pipeline(kind: str, payload: dict) -> dict:
    """Resolve the plugin for `kind` and process one payload.

    handle() is synchronous CPU work; it runs in the default executor
    so a slow plugin cannot stall the event loop.
    """
    plugin = resolve(kind)
    loop = asyncio.get_running_loop()
    return await loop.run_in_executor(None, plugin.handle, payload)


async def run_batch(kind: str, payloads: list[dict]) -> list[dict]:
    tasks = [run_pipeline(kind, p) for p in payloads]
    return list(await asyncio.gather(*tasks))
