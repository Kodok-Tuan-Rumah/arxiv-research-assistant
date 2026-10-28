import re

_VERSION_SUFFIX = re.compile(r"v\d+$")
_VALID = re.compile(r"^\d{4}\.\d{4,5}$")


def normalize_arxiv_id(value: str) -> str:
    if not isinstance(value, str):
        raise ValueError(f"invalid arxiv id: {value!r}")

    canonical = _VERSION_SUFFIX.sub("", value.strip())
    if not is_valid_arxiv_id(canonical):
        raise ValueError(f"invalid arxiv id: {value!r}")
    return canonical


def is_valid_arxiv_id(value: str) -> bool:
    return isinstance(value, str) and _VALID.fullmatch(value) is not None
