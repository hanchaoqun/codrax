"""Plugin base class and cooperative mixins.

handle() is a cooperative-super chain: each mixin does its part and
delegates with super().handle(payload), so the MRO order decides the
wrapping order.
"""

import abc
import time


class BasePlugin(abc.ABC):
    """Terminal link of the cooperative chain."""

    def handle(self, payload: dict) -> dict:
        # Base implementation: mark the payload processed. Mixins
        # earlier in the MRO have already run by the time this runs.
        out = dict(payload)
        out["processed"] = True
        return out

    @abc.abstractmethod
    def content_type(self) -> str:
        ...


class ValidationMixin:
    """Rejects payloads without a 'body' key before delegating."""

    def handle(self, payload: dict) -> dict:
        if "body" not in payload:
            raise ValueError("payload missing 'body'")
        return super().handle(payload)


class TimestampMixin:
    """Stamps ingestion time, then delegates down the MRO."""

    def handle(self, payload: dict) -> dict:
        stamped = dict(payload)
        stamped["ingested_at"] = time.time()
        return super().handle(stamped)
