try:
    import _fastlex  # native module exported by core-rs via pyo3
    _HAVE_NATIVE = True
except ImportError:  # pragma: no cover - fallback path
    _fastlex = None
    _HAVE_NATIVE = False


class FastTokenizer:
    """Facade over the Rust core. tokenize() routes to the native
    _fastlex.tokenize_bytes when available, else the slow pure-python
    merge loop below (kept semantically identical)."""

    def __init__(self, merges):
        self._merges = list(merges)
        self._rank = {(a, b): r for a, b, r in self._merges}

    def tokenize(self, text: str):
        data = text.encode("utf-8")
        if _HAVE_NATIVE:
            return _fastlex.tokenize_bytes(list(data), self._merges)
        return self._tokenize_slow(data)

    def _tokenize_slow(self, data: bytes):
        ids = list(data)
        while True:
            best = None
            for i in range(len(ids) - 1):
                rank = self._rank.get((ids[i], ids[i + 1]))
                if rank is not None and (best is None or rank < best[1]):
                    best = (i, rank)
            if best is None:
                return ids
            i, rank = best
            ids[i] = rank
            del ids[i + 1]
