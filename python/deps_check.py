"""Runtime check that third-party deps match the versions the app was
developed against. Keep EXPECTED in sync with requirements.txt.

Used by content_list_generator.py to render a non-blocking banner in
the GUI when an admin-deployed machine has drifted versions.
"""

from __future__ import annotations

from typing import List, NamedTuple


EXPECTED = {
    "customtkinter": "6.0.0",
    "blake3": "1.0.9",
}


class DepStatus(NamedTuple):
    name: str
    expected: str
    actual: str


def check_deps() -> List[DepStatus]:
    mismatches: List[DepStatus] = []
    for name, want in EXPECTED.items():
        try:
            mod = __import__(name)
        except ImportError:
            mismatches.append(DepStatus(name, want, "missing"))
            continue
        got = getattr(mod, "__version__", None)
        if got is None:
            try:
                from importlib.metadata import version as _meta_version
                got = _meta_version(name)
            except Exception:
                got = "unknown"
        if got != want:
            mismatches.append(DepStatus(name, want, got))
    return mismatches


def format_banner_text(mismatches: List[DepStatus]) -> str:
    lines = ["Dependency mismatch — contact IT to update:"]
    for ds in mismatches:
        lines.append(f"  {ds.name}: have {ds.actual}, expected {ds.expected}")
    return "\n".join(lines)
