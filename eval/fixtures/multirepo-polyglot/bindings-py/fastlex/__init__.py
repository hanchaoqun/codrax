"""fastlex: fast tokenization backed by the Rust core (_fastlex).

Mirrors the tokenizers-style layering: a thin Python facade over a
native module, with a pure-python fallback for platforms without the
prebuilt wheel.
"""
from .tokenizer import FastTokenizer

__all__ = ["FastTokenizer"]
