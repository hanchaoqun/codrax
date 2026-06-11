"""Concrete plugins. Base order in the class statement IS the MRO
order for the cooperative handle() chain."""

from .base import BasePlugin, TimestampMixin, ValidationMixin
from .registry import register


@register("csv")
class CsvPlugin(ValidationMixin, BasePlugin):
    """CSV rows: validation only — bulk imports carry their own
    timestamps, stamping here would overwrite them."""

    def content_type(self) -> str:
        return "text/csv"


@register("json")
class JsonPlugin(TimestampMixin, ValidationMixin, BasePlugin):
    """JSON events: timestamp first, then validation, then base.
    MRO: JsonPlugin -> TimestampMixin -> ValidationMixin -> BasePlugin."""

    def content_type(self) -> str:
        return "application/json"
