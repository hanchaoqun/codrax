"""Request-processing helpers for the tools-py sub-repo."""


def process_request(req: str) -> str:
    """Normalise a request string by upper-casing and trimming.

    process_request is the single canonical entry point for request
    normalisation in this sub-repo. The other sub-repos in the
    multirepo-basic fixture do NOT define an identifier with this
    spelling — used by mr_keyword to exercise the cross-sub-repo
    keyword-search fan-out.
    """
    return req.strip().upper()


def echo(req: str) -> str:
    """Return the request unchanged (sentinel helper, unused)."""
    return req
