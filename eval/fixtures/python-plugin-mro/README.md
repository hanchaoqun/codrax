# python-plugin-mro — decorator registry + cooperative MRO fixture

Blueprint shapes: pluggy-style plugin registry; flask extension
registration; cooperative-super mixin pipelines.

Design facts (cases assert exactly these):
- @register("name") (pipeline/registry.py) inserts the class into
  REGISTRY at import time; resolve(name) instantiates from REGISTRY.
- "json" resolves to JsonPlugin; "csv" resolves to CsvPlugin.
- JsonPlugin MRO: JsonPlugin -> TimestampMixin -> ValidationMixin ->
  BasePlugin (cooperative super().handle() chain runs in that order:
  timestamp wraps validation wraps base).
- CsvPlugin uses only ValidationMixin (no timestamp).
- run_pipeline (pipeline/runner.py) is async: resolve -> await
  plugin.handle(payload).
