"""Plugin registry: name -> plugin class.

Classes self-register at import time via the @register decorator, so
the registry is complete as soon as pipeline.plugins is imported —
no explicit setup call.
"""

REGISTRY: dict[str, type] = {}


def register(name: str):
    """Class decorator: bind `name` to the decorated plugin class."""

    def bind(cls: type) -> type:
        if name in REGISTRY:
            raise ValueError(f"duplicate plugin name: {name}")
        REGISTRY[name] = cls
        cls.plugin_name = name
        return cls

    return bind


def resolve(name: str):
    """Instantiate the plugin registered under `name`.

    Raises KeyError for unknown names — callers fail loud at setup
    instead of silently processing nothing.
    """
    try:
        cls = REGISTRY[name]
    except KeyError:
        raise KeyError(f"no plugin registered for {name!r}") from None
    return cls()
