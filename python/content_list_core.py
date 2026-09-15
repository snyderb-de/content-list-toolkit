from __future__ import annotations

import csv
import hashlib
import os
import re
import shutil
import time
import zipfile
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path
from typing import Callable
from xml.sax.saxutils import escape

try:
    import blake3
except Exception:  # pragma: no cover
    blake3 = None


SYSTEM_FILES = {".ds_store", "thumbs.db", "desktop.ini", "ehthumbs.db"}

# Always-excluded OS infrastructure — never archival content, no user toggle.
OS_NOISE_DIRS = {
    "$recycle.bin",
    "system volume information",
    ".spotlight-v100",
    ".trashes",
    ".fseventsd",
    ".temporaryitems",
    ".documentrevisions-v100",
}
OS_NOISE_FILES_EXACT = {
    "pagefile.sys", "hiberfil.sys", "swapfile.sys",
    "thumbs.db", "ehthumbs.db", "desktop.ini", ".ds_store",
}
OS_NOISE_PREFIXES = ("~$", ".~lock.", "._")
OS_NOISE_SUFFIXES = (".tmp",)
EMAIL_EXTENSIONS = {
    ".dbx",
    ".eml",
    ".emlx",
    ".emlxpart",
    ".mbox",
    ".mbx",
    ".msg",
    ".olk14msgsource",
    ".olk15message",
    ".ost",
    ".pst",
    ".rge",
    ".tbb",
    ".wdseml",
}
FOLDER_LIST_HEADERS = ["Path From Root Folder"]
HASH_ALGORITHM_OFF = "off"
HASH_ALGORITHM_BLAKE3 = "blake3"
HASH_ALGORITHM_SHA1 = "sha1"
HASH_ALGORITHM_SHA256 = "sha256"
DEFAULT_MAX_ROWS_PER_CSV = 300_000
UNKNOWN_FILE_TIMESTAMP = "unknown"
REPORT_HEADERS = [
    "File Name",
    "Extension",
    "Size in Bytes",
    "Size in Human Readable",
    "Path From Root Folder",
    "Hash Algorithm",
    "Hash Value",
    "Date Created",
    "Date Modified",
]
AGENCY_TEMPLATE_HEADERS = [
    "RG",
    "SubGr",
    "Series",
    "Sub_Series",
    "RC_Series",
    "Dept_Organization",
    "Division",
    "Section",
    "Unit",
    "RC_Series_Name",
    "Begin_Date",
    "End_Date",
    "File_Num",
    "File_Name",
    "Description",
    "Location",
    "First_Name",
    "Middle_Name",
    "Last_Name",
    "Date_Of_Birth",
    "File_Type",
    "Material_Type",
    "File_Format",
    "Comments",
    "Confidential",
    "Disposition_Date",
    "Box_Num",
    "Barcode",
    "TD_Num",
    "Location_ID",
    "Record_Level",
]
EMAIL_MANIFEST_HEADERS = [
    "Source Path",
    "Destination Path",
    "Relative Path",
    "File Name",
    "Extension",
    "Size in Bytes",
]
CLONE_DIFF_HEADERS = [
    "Difference Type",
    "1st Drive Path From Root Folder",
    "1st Drive File Name",
    "2nd Drive Path From Root Folder",
    "2nd Drive File Name",
    "1st Drive Size in Bytes",
    "2nd Drive Size in Bytes",
    "1st Drive Hash Algorithm",
    "2nd Drive Hash Algorithm",
    "1st Drive Hash Value",
    "2nd Drive Hash Value",
]

CLONE_VERDICT_EXACT    = "Exact Clone"
CLONE_VERDICT_CONTENT  = "Content Clone"
CLONE_VERDICT_METADATA = "Metadata Clone"
CLONE_VERDICT_NOT      = "Not a Clone"

_PDF_ID_RE = re.compile(rb'/ID\s*\[<[0-9a-fA-F]+><[0-9a-fA-F]+>\]')
_PDF_SOFT_TAIL = 2048


def format_file_timestamp(value: float | None) -> str:
    if value is None or value == 0:
        return UNKNOWN_FILE_TIMESTAMP
    try:
        local_value = datetime.fromtimestamp(value).astimezone()
        offset = local_value.utcoffset()
        if offset is None:
            return UNKNOWN_FILE_TIMESTAMP
        offset_seconds = int(offset.total_seconds())
    except (OSError, OverflowError, TypeError, ValueError):
        return UNKNOWN_FILE_TIMESTAMP
    sign = "+" if offset_seconds >= 0 else "-"
    offset_minutes = abs(offset_seconds) // 60
    offset_hours, offset_minutes = divmod(offset_minutes, 60)
    return (
        f"{local_value.year:04d}-{local_value.month:02d}-{local_value.day:02d} "
        f"{local_value.hour:02d}:{local_value.minute:02d}:{local_value.second:02d} "
        f"{sign}{offset_hours:02d}:{offset_minutes:02d}"
    )


def file_creation_timestamp(stat_result: os.stat_result) -> float | None:
    birth_time = getattr(stat_result, "st_birthtime", None)
    if birth_time is not None:
        try:
            return float(birth_time)
        except (OverflowError, TypeError, ValueError):
            return None
    if os.name == "nt":
        windows_creation_time = getattr(stat_result, "st_ctime", None)
        if windows_creation_time is not None:
            try:
                return float(windows_creation_time)
            except (OverflowError, TypeError, ValueError):
                return None
    return None


def _pdf_normalized_tail(path: Path) -> bytes | None:
    try:
        size = path.stat().st_size
        offset = max(0, size - _PDF_SOFT_TAIL)
        with path.open("rb") as f:
            f.seek(offset)
            tail = f.read(_PDF_SOFT_TAIL)
        return _PDF_ID_RE.sub(b"/ID[<0><0>]", tail)
    except OSError:
        return None


def _pdf_soft_match(path_a: Path, path_b: Path) -> bool:
    tail_a = _pdf_normalized_tail(path_a)
    tail_b = _pdf_normalized_tail(path_b)
    return tail_a is not None and tail_b is not None and tail_a == tail_b


def _soft_index_key(file_name: str, size: int) -> str:
    return f"{file_name}::{size}"


@dataclass
class SummaryEntry:
    label: str
    count: int
    bytes: int


@dataclass
class ScanResult:
    source_name: str
    source_dir: Path
    output_path: Path
    output_paths: list[Path]
    xlsx_path: Path | None
    xlsx_paths: list[Path]
    report_path: Path
    files: int
    directories: int
    total_bytes: int
    filtered: int
    filtered_hidden: int
    filtered_system: int
    filtered_exts: int
    filtered_os_noise: int
    elapsed: float
    hash_algorithm: str
    create_xlsx: bool
    preserve_zeros: bool
    delete_csv: bool
    csv_deleted: bool
    max_rows_per_csv: int
    csv_parts: int
    xlsx_parts: int
    hash_workers: int
    top_by_count: list[SummaryEntry]
    top_by_size: list[SummaryEntry]
    first_csv_item: str
    last_csv_item: str
    folders_only: bool = False
    agency_template: bool = False


@dataclass
class CloneCompareProgress:
    compared: int
    total: int
    differences: int
    current_name: str = ""


@dataclass
class CloneVerificationResult:
    drive_a: ScanResult
    drive_b: ScanResult
    diff_path: Path
    report_path: Path
    hash_algorithm: str
    elapsed: float
    verdict: str
    compared: int
    differences: int
    moved_files: int
    duplicates_on_b: int
    duplicates_on_a: int
    missing_no_match: int
    extra_no_match: int
    size_mismatches: int
    hash_mismatches: int
    excluded_system: int
    metadata_only_diffs: int = 0
    soft_compare: bool = False


@dataclass
class EmailCopyResult:
    source_dir: Path
    dest_dir: Path
    manifest_path: Path
    copied: int
    elapsed: float


@dataclass
class EmailCopyProgress:
    phase: str
    scanned: int
    matched: int
    copied: int
    total: int
    current_relative: str = ""
    current_name: str = ""


@dataclass
class ScanProgress:
    phase: str
    files: int
    total_files: int
    directories: int
    total_directories: int
    bytes: int
    total_bytes: int
    filtered: int
    current_name: str = ""


class ScanCanceled(Exception):
    pass


@dataclass
class ScanCSVRow:
    file_name: str
    extension: str
    size: int
    relative_path: str
    hash_algorithm: str
    hash_value: str


def normalize_exts(raw: str) -> set[str]:
    result: set[str] = set()
    for part in raw.split(","):
        clean = part.strip().lower().lstrip(".")
        if clean:
            result.add(clean)
    return result


def default_output_name(source_dir: Path) -> str:
    stamp = time.strftime("%Y-%m-%dT%H-%M-%S")
    base = source_dir.name or "content-list"
    return f"{base}-content-list-{stamp}.csv"


def default_folder_list_output_name(source_dir: Path) -> str:
    stamp = time.strftime("%Y-%m-%dT%H-%M-%S")
    base = source_dir.name or "folder-list"
    return f"{base}-folder-list-{stamp}.csv"


def default_manifest_name() -> str:
    return f"email-copy-manifest-{time.strftime('%Y-%m-%dT%H-%M-%S')}.csv"


def normalize_extension(path: Path) -> str:
    return path.suffix.lower().lstrip(".")


def default_hash_algorithm() -> str:
    return HASH_ALGORITHM_SHA1


def is_blake3_available() -> bool:
    return blake3 is not None


def normalize_hash_algorithm(value: str | None) -> str:
    normalized = str(value or "").strip().lower()
    if normalized in {"", "off", "none"}:
        return HASH_ALGORITHM_OFF
    if normalized in {"blake3", "fast", "fast (blake3)"}:
        return HASH_ALGORITHM_BLAKE3
    if normalized in {"sha1", "sha-1", "medium", "medium (sha-1)"}:
        return HASH_ALGORITHM_SHA1
    if normalized in {"sha256", "sha-256", "strong", "strong (sha-256)"}:
        return HASH_ALGORITHM_SHA256
    return default_hash_algorithm()


def hash_algorithm_label(value: str) -> str:
    normalized = normalize_hash_algorithm(value)
    if normalized == HASH_ALGORITHM_BLAKE3:
        return "Fast (BLAKE3)"
    if normalized == HASH_ALGORITHM_SHA1:
        return "Medium (SHA-1)"
    if normalized == HASH_ALGORITHM_SHA256:
        return "Strong (SHA-256)"
    return "No hash"


def hash_algorithm_csv_name(value: str) -> str:
    normalized = normalize_hash_algorithm(value)
    if normalized == HASH_ALGORITHM_BLAKE3:
        return "BLAKE3"
    if normalized == HASH_ALGORITHM_SHA1:
        return "SHA-1"
    if normalized == HASH_ALGORITHM_SHA256:
        return "SHA-256"
    return ""


def hash_algorithm_labels() -> list[str]:
    return [
        hash_algorithm_label(HASH_ALGORITHM_OFF),
        hash_algorithm_label(HASH_ALGORITHM_BLAKE3),
        hash_algorithm_label(HASH_ALGORITHM_SHA1),
        hash_algorithm_label(HASH_ALGORITHM_SHA256),
    ]


def summary_key(ext: str) -> str:
    return f".{ext}" if ext else "[no extension]"


def is_always_excluded_dir(name: str) -> bool:
    return name.lower() in OS_NOISE_DIRS


def is_always_excluded_file(name: str) -> bool:
    lower = name.lower()
    if lower in OS_NOISE_FILES_EXACT:
        return True
    if any(lower.startswith(p) for p in OS_NOISE_PREFIXES if p != "._"):
        return True
    if name.startswith("._"):  # macOS resource forks — case-sensitive prefix
        return True
    if any(lower.endswith(s) for s in OS_NOISE_SUFFIXES):
        return True
    return False


def is_hidden_path(path: Path, source_root: Path) -> bool:
    relative = path.relative_to(source_root)
    return any(part.startswith(".") for part in relative.parts)


def should_skip(
    path: Path,
    source_root: Path,
    include_hidden: bool,
    include_system: bool,
    excluded_exts: set[str],
) -> tuple[str, bool]:
    if is_always_excluded_file(path.name):
        return "os noise", True
    if not include_hidden and is_hidden_path(path, source_root):
        return "hidden path", True
    if not include_system and path.name.lower() in SYSTEM_FILES:
        return "system file", True
    if normalize_extension(path) in excluded_exts:
        return "excluded extension", True
    return "", False


def hash_file(path: Path, algorithm: str, cancel_event=None) -> str:
    normalized = normalize_hash_algorithm(algorithm)
    if normalized == HASH_ALGORITHM_OFF:
        return ""
    if normalized == HASH_ALGORITHM_BLAKE3:
        if blake3 is None:
            raise RuntimeError("BLAKE3 hashing requires the 'blake3' Python package. Install it with: pip install -r requirements.txt")
        digest = blake3.blake3()
    elif normalized == HASH_ALGORITHM_SHA1:
        digest = hashlib.sha1()
    else:
        digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            if cancel_event is not None and cancel_event.is_set():
                raise ScanCanceled()
            digest.update(chunk)
    return digest.hexdigest()


def human_bytes(size: int) -> str:
    units = ["B", "KB", "MB", "GB", "TB", "PB"]
    value = float(size)
    index = 0
    while value >= 1024 and index < len(units) - 1:
        value /= 1024
        index += 1
    if index == 0:
        return f"{size} {units[index]}"
    if value >= 10:
        return f"{value:.1f} {units[index]}"
    return f"{value:.2f} {units[index]}"


def folder_display_name(path: Path) -> str:
    name = path.name
    return name or str(path)


def count_scan_targets(
    source_dir: Path,
    include_hidden: bool,
    include_system: bool,
    excluded_exts: set[str],
    progress_callback: Callable[[ScanProgress], None] | None = None,
    cancel_event=None,
) -> tuple[int, int, int, int, int, int, int]:
    files = 0
    filtered = 0
    filtered_hidden = 0
    filtered_system = 0
    filtered_exts = 0
    filtered_os_noise = 0
    directories = 0
    total_bytes = 0

    for root, dirs, names in os.walk(source_dir):
        if cancel_event is not None and cancel_event.is_set():
            raise ScanCanceled()
        root_path = Path(root)
        directories += 1

        kept_dirs: list[str] = []
        for directory in dirs:
            if is_always_excluded_dir(directory):
                filtered += 1
                filtered_os_noise += 1
                continue
            candidate = root_path / directory
            if not include_hidden and is_hidden_path(candidate, source_dir):
                filtered += 1
                filtered_hidden += 1
                continue
            kept_dirs.append(directory)
        dirs[:] = sorted(kept_dirs)

        if progress_callback is not None and directories % 100 == 0:
            progress_callback(
                ScanProgress(
                    phase="counting",
                    files=files,
                    total_files=0,
                    directories=directories,
                    total_directories=0,
                    bytes=total_bytes,
                    total_bytes=0,
                    filtered=filtered,
                    current_name=root_path.name,
                )
            )

        for file_name in sorted(names):
            if cancel_event is not None and cancel_event.is_set():
                raise ScanCanceled()
            candidate = root_path / file_name
            reason, skipped = should_skip(candidate, source_dir, include_hidden, include_system, excluded_exts)
            if skipped:
                filtered += 1
                if reason == "hidden path":
                    filtered_hidden += 1
                elif reason == "system file":
                    filtered_system += 1
                elif reason == "excluded extension":
                    filtered_exts += 1
                elif reason == "os noise":
                    filtered_os_noise += 1
                continue
            try:
                stat = candidate.stat()
            except OSError:
                continue
            if not candidate.is_file():
                continue
            files += 1
            total_bytes += stat.st_size
            if progress_callback is not None and files % 250 == 0:
                progress_callback(
                    ScanProgress(
                        phase="counting",
                        files=files,
                        total_files=0,
                        directories=directories,
                        total_directories=0,
                        bytes=total_bytes,
                        total_bytes=0,
                        filtered=filtered,
                        current_name=file_name,
                    )
                )

    if progress_callback is not None:
        progress_callback(
            ScanProgress(
                phase="counting",
                files=files,
                total_files=0,
                directories=directories,
                total_directories=0,
                bytes=total_bytes,
                total_bytes=0,
                filtered=filtered,
            )
        )
    return files, filtered, filtered_hidden, filtered_system, filtered_exts, filtered_os_noise, directories, total_bytes


def iter_scan_files(
    source_dir: Path,
    include_hidden: bool,
    include_system: bool,
    excluded_exts: set[str],
    cancel_event=None,
):
    for root, dirs, names in os.walk(source_dir):
        if cancel_event is not None and cancel_event.is_set():
            raise ScanCanceled()
        root_path = Path(root)

        kept_dirs: list[str] = []
        for directory in dirs:
            if is_always_excluded_dir(directory):
                continue
            candidate = root_path / directory
            if not include_hidden and is_hidden_path(candidate, source_dir):
                continue
            kept_dirs.append(directory)
        dirs[:] = sorted(kept_dirs)

        for file_name in sorted(names):
            if cancel_event is not None and cancel_event.is_set():
                raise ScanCanceled()
            candidate = root_path / file_name
            _, skipped = should_skip(candidate, source_dir, include_hidden, include_system, excluded_exts)
            if skipped:
                continue
            try:
                stat = candidate.stat()
            except OSError:
                continue
            if not candidate.is_file():
                continue
            yield (
                candidate,
                stat.st_size,
                candidate.relative_to(source_dir).as_posix(),
                format_file_timestamp(file_creation_timestamp(stat)),
                format_file_timestamp(getattr(stat, "st_mtime", None)),
            )


def _folder_rel_depth(rel: str) -> int:
    if not rel or rel == ".":
        return 0
    return rel.count("/") + 1


def count_scan_dirs(
    source_dir: Path,
    include_hidden: bool,
    max_depth: int = 0,
    progress_callback: Callable[[ScanProgress], None] | None = None,
    cancel_event=None,
) -> int:
    directories = 0
    for root, dirs, _names in os.walk(source_dir):
        if cancel_event is not None and cancel_event.is_set():
            raise ScanCanceled()
        root_path = Path(root)
        rel = root_path.relative_to(source_dir).as_posix() if root_path != source_dir else ""
        current_depth = _folder_rel_depth(rel)
        kept = []
        for d in dirs:
            if is_always_excluded_dir(d):
                continue
            candidate = root_path / d
            if not include_hidden and is_hidden_path(candidate, source_dir):
                continue
            child_depth = current_depth + 1
            if max_depth > 0 and child_depth > max_depth:
                continue
            kept.append(d)
        dirs[:] = sorted(kept)
        if root_path == source_dir:
            continue
        directories += 1
        if progress_callback is not None and directories % 100 == 0:
            progress_callback(
                ScanProgress(
                    phase="counting",
                    files=0,
                    total_files=0,
                    directories=directories,
                    total_directories=0,
                    bytes=0,
                    total_bytes=0,
                    filtered=0,
                    current_name=root_path.name,
                )
            )
    return directories


def iter_scan_dirs(
    source_dir: Path,
    include_hidden: bool,
    max_depth: int = 0,
    cancel_event=None,
):
    for root, dirs, _names in os.walk(source_dir):
        if cancel_event is not None and cancel_event.is_set():
            raise ScanCanceled()
        root_path = Path(root)
        rel = root_path.relative_to(source_dir).as_posix() if root_path != source_dir else ""
        current_depth = _folder_rel_depth(rel)
        kept = []
        for d in dirs:
            if is_always_excluded_dir(d):
                continue
            candidate = root_path / d
            if not include_hidden and is_hidden_path(candidate, source_dir):
                continue
            child_depth = current_depth + 1
            if max_depth > 0 and child_depth > max_depth:
                continue
            kept.append(d)
        dirs[:] = sorted(kept)
        if root_path == source_dir:
            continue
        yield rel


def write_folder_list_csv(
    source_dir: Path,
    output_path: Path,
    *,
    include_hidden: bool,
    max_depth: int = 0,
    total_dirs: int,
    progress_callback: Callable[[ScanProgress], None] | None = None,
    cancel_event=None,
) -> tuple[int, str, str, list[Path]]:
    part_path = csv_output_path_for_part(output_path, 1)
    part_path.parent.mkdir(parents=True, exist_ok=True)
    dirs_written = 0
    first_item = ""
    last_item = ""
    with part_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(FOLDER_LIST_HEADERS)
        for rel in iter_scan_dirs(source_dir, include_hidden, max_depth=max_depth, cancel_event=cancel_event):
            writer.writerow([rel])
            dirs_written += 1
            if not first_item:
                first_item = rel
            last_item = rel
            if progress_callback is not None:
                progress_callback(
                    ScanProgress(
                        phase="scanning",
                        files=dirs_written,
                        total_files=total_dirs,
                        directories=dirs_written,
                        total_directories=total_dirs,
                        bytes=0,
                        total_bytes=0,
                        filtered=0,
                        current_name=rel.split("/")[-1],
                    )
                )
    return dirs_written, first_item, last_item, [part_path]


def summarize_entries(summaries: dict[str, dict[str, int]], key: str) -> list[SummaryEntry]:
    entries = [
        SummaryEntry(label=label, count=data["count"], bytes=data["bytes"])
        for label, data in summaries.items()
    ]
    if key == "count":
        entries.sort(key=lambda item: (-item.count, -item.bytes, item.label))
    else:
        entries.sort(key=lambda item: (-item.bytes, -item.count, item.label))
    return entries[:8]


def csv_output_path_for_part(output_path: Path, part_number: int) -> Path:
    part_number = max(1, int(part_number))
    stem = output_path.stem
    suffix = output_path.suffix or ".csv"
    return output_path.with_name(f"{stem}-{part_number:03d}{suffix}")


def clone_output_path_for_drive_b(output_path: Path) -> Path:
    return output_path.with_name(f"{output_path.stem}-clone-b{output_path.suffix or '.csv'}")


def clone_diff_csv_path(output_path: Path) -> Path:
    return output_path.with_name(f"{output_path.stem}-clone-differences{output_path.suffix or '.csv'}")


def clone_diff_report_path(output_path: Path) -> Path:
    return output_path.with_name(f"{output_path.stem}-clone-report.txt")


def compare_progress_fraction(progress: CloneCompareProgress) -> float:
    if progress.total <= 0:
        return 0.0
    value = progress.compared / max(1, progress.total)
    return max(0.0, min(1.0, value))


def iter_scan_csv_rows(paths: list[Path]):
    for path in paths:
        with path.open("r", newline="", encoding="utf-8") as handle:
            reader = csv.reader(handle)
            next(reader, None)
            for row in reader:
                if len(row) < 7:
                    raise RuntimeError(f"Scan CSV row in {path} is missing columns")
                try:
                    size = int(str(row[2]).strip())
                except ValueError as exc:
                    raise RuntimeError(f"Scan CSV row in {path} has invalid size: {row[2]!r}") from exc
                yield ScanCSVRow(
                    file_name=row[0],
                    extension=row[1],
                    size=size,
                    relative_path=row[4],
                    hash_algorithm=row[5],
                    hash_value=row[6],
                )


class ChunkedCSVReportWriter:
    def __init__(self, output_path: Path, max_rows_per_csv: int, headers: list[str] | None = None) -> None:
        self.output_path = output_path
        self.max_rows_per_csv = max(1, max_rows_per_csv)
        self.headers = list(headers or REPORT_HEADERS)
        self.part_paths: list[Path] = []
        self._part_number = 0
        self._rows_in_part = 0
        self._handle = None
        self._writer = None
        self._open_next_part()

    def _path_for_part(self, part_number: int) -> Path:
        return csv_output_path_for_part(self.output_path, part_number)

    def _open_next_part(self) -> None:
        self.close()
        self._part_number += 1
        part_path = self._path_for_part(self._part_number)
        part_path.parent.mkdir(parents=True, exist_ok=True)
        self._handle = part_path.open("w", newline="", encoding="utf-8")
        self._writer = csv.writer(self._handle)
        self._writer.writerow(self.headers)
        self._rows_in_part = 0
        self.part_paths.append(part_path)

    def write_row(self, row: list[str]) -> None:
        if self._writer is None:
            raise RuntimeError("CSV writer is not initialized")
        if self._rows_in_part >= self.max_rows_per_csv:
            self._open_next_part()
        self._writer.writerow(row)
        self._rows_in_part += 1

    def close(self) -> None:
        if self._handle is not None:
            self._handle.close()
        self._handle = None
        self._writer = None


def _agency_field(fields: dict[str, str] | None, key: str, default: str = "") -> str:
    if not fields:
        return default
    value = fields.get(key, default)
    return str(value) if value is not None else default


def _agency_file_format(ext: str) -> str:
    return ext.removeprefix(".").upper()


def _agency_location(fields: dict[str, str] | None, relative: str) -> str:
    override = _agency_field(fields, "location").strip()
    if override:
        return override
    parent = str(Path(relative).parent).replace("\\", "/")
    return "" if parent == "." else parent


def normalize_agency_template_fields(agency_fields: dict[str, str] | None) -> dict[str, str]:
    normalized = dict(agency_fields or {})
    normalized["rg"] = _normalize_agency_code("RG", normalized.get("rg", ""), 4)
    normalized["sg"] = _normalize_agency_code("SG", normalized.get("sg", ""), 3)
    normalized["series"] = _normalize_agency_code("Series", normalized.get("series", ""), 3)
    return normalized


def _normalize_agency_code(label: str, value: object, width: int) -> str:
    text = "" if value is None else str(value).strip()
    if not text:
        return ""
    if not text.isdigit():
        raise ValueError(f"{label} must contain only digits")
    if len(text) > width:
        raise ValueError(f"{label} must be {width} digits or fewer")
    return text.zfill(width)


def agency_template_row(
    file_name: str,
    ext: str,
    relative: str,
    agency_fields: dict[str, str] | None,
) -> list[str]:
    agency_fields = normalize_agency_template_fields(agency_fields)
    comments = _agency_field(agency_fields, "comments").strip()
    material_type = _agency_field(agency_fields, "material_type", "Born Digital").strip() or "Born Digital"
    record_level = _agency_field(agency_fields, "record_level", "Item").strip() or "Item"
    return [
        _agency_field(agency_fields, "rg"),
        _agency_field(agency_fields, "sg"),
        _agency_field(agency_fields, "series"),
        "",
        _agency_field(agency_fields, "rc_series"),
        _agency_field(agency_fields, "dept_organization"),
        _agency_field(agency_fields, "division"),
        _agency_field(agency_fields, "section"),
        _agency_field(agency_fields, "unit"),
        _agency_field(agency_fields, "rc_series_name"),
        _agency_field(agency_fields, "begin_date"),
        _agency_field(agency_fields, "end_date"),
        "",
        file_name,
        _agency_field(agency_fields, "description"),
        _agency_location(agency_fields, relative),
        "",
        "",
        "",
        "",
        "",
        material_type,
        _agency_file_format(ext),
        comments,
        _agency_field(agency_fields, "confidential"),
        _agency_field(agency_fields, "disposition_date"),
        _agency_field(agency_fields, "box_num"),
        "",
        _agency_field(agency_fields, "td_num"),
        _agency_field(agency_fields, "location_id"),
        record_level,
    ]


def write_csv_report(
    source_dir: Path,
    output_path: Path,
    *,
    hash_algorithm: str,
    include_hidden: bool,
    include_system: bool,
    excluded_exts: set[str],
    max_rows_per_csv: int,
    total_expected_files: int,
    total_directories: int,
    filtered: int,
    total_expected_bytes: int,
    agency_template: bool = False,
    agency_fields: dict[str, str] | None = None,
    progress_callback: Callable[[ScanProgress], None] | None = None,
    cancel_event=None,
) -> tuple[int, int, dict[str, dict[str, int]], int, str, str, list[Path]]:
    summaries: dict[str, dict[str, int]] = {}
    processed_bytes = 0
    processed_files = 0
    first_csv_item = ""
    last_csv_item = ""
    hash_workers = 1
    normalized_algorithm = normalize_hash_algorithm(hash_algorithm)
    headers = AGENCY_TEMPLATE_HEADERS if agency_template else REPORT_HEADERS
    csv_writer = ChunkedCSVReportWriter(output_path, max_rows_per_csv, headers)

    def write_processed_row(
        file_path: Path,
        size: int,
        relative: str,
        date_created: str,
        date_modified: str,
        file_hash: str,
    ) -> None:
        nonlocal processed_bytes, processed_files, first_csv_item, last_csv_item
        ext = normalize_extension(file_path)
        bucket = summaries.setdefault(summary_key(ext), {"count": 0, "bytes": 0})
        bucket["count"] += 1
        bucket["bytes"] += size
        row = [
            file_path.name,
            ext,
            str(size),
            human_bytes(size),
            relative,
            hash_algorithm_csv_name(normalized_algorithm),
            file_hash,
            date_created,
            date_modified,
        ]
        if agency_template:
            row = agency_template_row(file_path.name, ext, relative, agency_fields)
        csv_writer.write_row(row)
        processed_files += 1
        processed_bytes += size
        if not first_csv_item:
            first_csv_item = relative
        last_csv_item = relative
        if progress_callback is not None:
            progress_callback(
                ScanProgress(
                    phase="scanning",
                    files=processed_files,
                    total_files=total_expected_files,
                    directories=total_directories,
                    total_directories=total_directories,
                    bytes=processed_bytes,
                    total_bytes=total_expected_bytes,
                    filtered=filtered,
                    current_name=file_path.name,
                )
            )

    try:
        if normalized_algorithm != HASH_ALGORITHM_OFF:
            hash_workers = max(2, os.cpu_count() or 2)
            max_in_flight = max(hash_workers*4, 8)
            with ThreadPoolExecutor(max_workers=hash_workers) as pool:
                pending = deque()
                for file_path, size, relative, date_created, date_modified in iter_scan_files(
                    source_dir,
                    include_hidden,
                    include_system,
                    excluded_exts,
                    cancel_event=cancel_event,
                ):
                    if cancel_event is not None and cancel_event.is_set():
                        raise ScanCanceled()
                    pending.append(
                        (
                            file_path,
                            size,
                            relative,
                            date_created,
                            date_modified,
                            pool.submit(hash_file, file_path, normalized_algorithm, cancel_event),
                        )
                    )
                    if len(pending) >= max_in_flight:
                        next_path, next_size, next_relative, next_created, next_modified, next_future = pending.popleft()
                        write_processed_row(
                            next_path,
                            next_size,
                            next_relative,
                            next_created,
                            next_modified,
                            next_future.result(),
                        )

                while pending:
                    next_path, next_size, next_relative, next_created, next_modified, next_future = pending.popleft()
                    write_processed_row(
                        next_path,
                        next_size,
                        next_relative,
                        next_created,
                        next_modified,
                        next_future.result(),
                    )
        else:
            for file_path, size, relative, date_created, date_modified in iter_scan_files(
                source_dir,
                include_hidden,
                include_system,
                excluded_exts,
                cancel_event=cancel_event,
            ):
                if cancel_event is not None and cancel_event.is_set():
                    raise ScanCanceled()
                write_processed_row(file_path, size, relative, date_created, date_modified, "")
    finally:
        csv_writer.close()

    return processed_files, processed_bytes, summaries, hash_workers, first_csv_item, last_csv_item, csv_writer.part_paths


def _xlsx_col_name(index: int) -> str:
    name = ""
    while index > 0:
        index, remainder = divmod(index - 1, 26)
        name = chr(ord("A") + remainder) + name
    return name


def _xlsx_cell_ref(row_index: int, col_index: int) -> str:
    return f"{_xlsx_col_name(col_index)}{row_index}"


def _xlsx_inline_cell(ref: str, value: str, style_id: int = 0) -> str:
    style_attr = f' s="{style_id}"' if style_id else ""
    return f'<c r="{ref}" t="inlineStr"{style_attr}><is><t>{escape(value)}</t></is></c>'


def _xlsx_number_cell(ref: str, value: str, style_id: int = 0) -> str:
    style_attr = f' s="{style_id}"' if style_id else ""
    return f'<c r="{ref}"{style_attr}><v>{escape(value)}</v></c>'


def convert_csv_to_xlsx(csv_path: Path, xlsx_path: Path, preserve_zeros: bool) -> None:
    xlsx_path.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(xlsx_path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        archive.writestr(
            "[Content_Types].xml",
            """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
  <Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
  <Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>""",
        )
        archive.writestr(
            "_rels/.rels",
            """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>""",
        )
        archive.writestr(
            "docProps/core.xml",
            """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <dc:title>Content List Toolkit</dc:title>
</cp:coreProperties>""",
        )
        archive.writestr(
            "docProps/app.xml",
            """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes">
  <Application>Content List Toolkit</Application>
</Properties>""",
        )
        archive.writestr(
            "xl/workbook.xml",
            """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Sheet1" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>""",
        )
        archive.writestr(
            "xl/_rels/workbook.xml.rels",
            """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>""",
        )
        archive.writestr(
            "xl/styles.xml",
            """<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <numFmts count="1">
    <numFmt numFmtId="49" formatCode="@"/>
  </numFmts>
  <fonts count="1">
    <font><sz val="11"/><name val="Calibri"/></font>
  </fonts>
  <fills count="2">
    <fill><patternFill patternType="none"/></fill>
    <fill><patternFill patternType="gray125"/></fill>
  </fills>
  <borders count="1">
    <border><left/><right/><top/><bottom/><diagonal/></border>
  </borders>
  <cellStyleXfs count="1">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0"/>
  </cellStyleXfs>
  <cellXfs count="2">
    <xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>
    <xf numFmtId="49" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>
  </cellXfs>
  <cellStyles count="1">
    <cellStyle name="Normal" xfId="0" builtinId="0"/>
  </cellStyles>
</styleSheet>""",
        )
        with csv_path.open("r", newline="", encoding="utf-8") as handle:
            reader = csv.reader(handle)
            with archive.open("xl/worksheets/sheet1.xml", "w") as sheet:
                sheet.write(
                    (
                        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
                        '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
                        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
                        '<sheetViews><sheetView workbookViewId="0"/></sheetViews>'
                        '<sheetFormatPr defaultRowHeight="15"/>'
                        "<sheetData>"
                    ).encode("utf-8")
                )
                for row_index, row in enumerate(reader, start=1):
                    cells: list[str] = []
                    for col_index, value in enumerate(row, start=1):
                        ref = _xlsx_cell_ref(row_index, col_index)
                        if preserve_zeros:
                            cells.append(_xlsx_inline_cell(ref, value, style_id=1))
                        elif row_index > 1 and col_index == 3 and value.isdigit():
                            cells.append(_xlsx_number_cell(ref, value))
                        else:
                            cells.append(_xlsx_inline_cell(ref, value))
                    row_xml = f'<row r="{row_index}">{"".join(cells)}</row>'
                    sheet.write(row_xml.encode("utf-8"))
                sheet.write(b"</sheetData></worksheet>")


def render_top(title: str, entries: list[SummaryEntry], by_size: bool = False) -> list[str]:
    lines = [title]
    for entry in entries:
        if by_size:
            lines.append(f"  {entry.label}: {human_bytes(entry.bytes)}, {entry.count} files")
        else:
            lines.append(f"  {entry.label}: {entry.count} files, {human_bytes(entry.bytes)}")
    if len(lines) == 1:
        lines.append("  No files were written.")
    return lines


def _run_folder_list_scan(
    source_dir: Path,
    output_path: Path,
    *,
    include_hidden: bool,
    folder_depth: int = 0,
    max_rows_per_csv: int = DEFAULT_MAX_ROWS_PER_CSV,
    progress_callback: Callable[[ScanProgress], None] | None = None,
    cancel_event=None,
) -> ScanResult:
    started = time.time()
    max_rows_per_csv = max(1, int(max_rows_per_csv or DEFAULT_MAX_ROWS_PER_CSV))
    report_path = output_path.with_name(f"{output_path.stem}-report.txt")
    csv_paths: list[Path] = []
    try:
        total_dirs = count_scan_dirs(
            source_dir,
            include_hidden,
            max_depth=folder_depth,
            progress_callback=progress_callback,
            cancel_event=cancel_event,
        )
        dirs_written, first_item, last_item, csv_paths = write_folder_list_csv(
            source_dir,
            output_path,
            include_hidden=include_hidden,
            max_depth=folder_depth,
            total_dirs=total_dirs,
            progress_callback=progress_callback,
            cancel_event=cancel_event,
        )
        result = ScanResult(
            source_name=folder_display_name(source_dir),
            source_dir=source_dir,
            output_path=csv_paths[0] if csv_paths else output_path,
            output_paths=csv_paths or [output_path],
            xlsx_path=None,
            xlsx_paths=[],
            report_path=report_path,
            files=0,
            directories=dirs_written,
            total_bytes=0,
            filtered=0,
            filtered_hidden=0,
            filtered_system=0,
            filtered_exts=0,
            filtered_os_noise=0,
            elapsed=time.time() - started,
            hash_algorithm=HASH_ALGORITHM_OFF,
            create_xlsx=False,
            preserve_zeros=False,
            delete_csv=False,
            csv_deleted=False,
            max_rows_per_csv=max_rows_per_csv,
            csv_parts=len(csv_paths or [output_path]),
            xlsx_parts=0,
            hash_workers=0,
            top_by_count=[],
            top_by_size=[],
            first_csv_item=first_item,
            last_csv_item=last_item,
            folders_only=True,
        )
        write_scan_report(result)
        return result
    except ScanCanceled:
        for path in csv_paths:
            try:
                path.unlink()
            except FileNotFoundError:
                pass
        raise


def run_scan(
    source_dir: Path,
    output_path: Path,
    *,
    hash_algorithm: str,
    include_hidden: bool,
    include_system: bool,
    excluded_exts: set[str],
    create_xlsx: bool,
    preserve_zeros: bool,
    delete_csv: bool,
    max_rows_per_csv: int = DEFAULT_MAX_ROWS_PER_CSV,
    folders_only: bool = False,
    folder_depth: int = 0,
    agency_template: bool = False,
    agency_fields: dict[str, str] | None = None,
    progress_callback: Callable[[ScanProgress], None] | None = None,
    cancel_event=None,
) -> ScanResult:
    if folders_only:
        return _run_folder_list_scan(
            source_dir,
            output_path,
            include_hidden=include_hidden,
            folder_depth=folder_depth,
            max_rows_per_csv=max_rows_per_csv,
            progress_callback=progress_callback,
            cancel_event=cancel_event,
        )
    if agency_template:
        agency_fields = normalize_agency_template_fields(agency_fields)
    started = time.time()
    max_rows_per_csv = max(1, int(max_rows_per_csv or DEFAULT_MAX_ROWS_PER_CSV))
    csv_paths: list[Path] = []
    xlsx_paths: list[Path] = []
    xlsx_path: Path | None = None
    csv_deleted = False
    report_path = output_path.with_name(f"{output_path.stem}-report.txt")
    try:
        total_files, filtered, filtered_hidden, filtered_system, filtered_exts, filtered_os_noise, directories, total_expected_bytes = count_scan_targets(
            source_dir,
            include_hidden,
            include_system,
            excluded_exts,
            progress_callback=progress_callback,
            cancel_event=cancel_event,
        )
        file_count, total_bytes, summaries, hash_workers, first_csv_item, last_csv_item, csv_paths = write_csv_report(
            source_dir,
            output_path,
            hash_algorithm=hash_algorithm,
            include_hidden=include_hidden,
            include_system=include_system,
            excluded_exts=excluded_exts,
            max_rows_per_csv=max_rows_per_csv,
            total_expected_files=total_files,
            total_directories=directories,
            filtered=filtered,
            total_expected_bytes=total_expected_bytes,
            agency_template=agency_template,
            agency_fields=agency_fields,
            progress_callback=progress_callback,
            cancel_event=cancel_event,
        )
        if create_xlsx:
            for csv_part_path in csv_paths:
                if cancel_event is not None and cancel_event.is_set():
                    raise ScanCanceled()
                xlsx_part_path = csv_part_path.with_suffix(".xlsx")
                convert_csv_to_xlsx(csv_part_path, xlsx_part_path, preserve_zeros)
                xlsx_paths.append(xlsx_part_path)
            if xlsx_paths:
                xlsx_path = xlsx_paths[0]
            if delete_csv:
                for csv_part_path in csv_paths:
                    csv_part_path.unlink(missing_ok=True)
                csv_deleted = True
        result = ScanResult(
            source_name=folder_display_name(source_dir),
            source_dir=source_dir,
            output_path=csv_paths[0] if csv_paths else output_path,
            output_paths=csv_paths or [output_path],
            xlsx_path=xlsx_path,
            xlsx_paths=xlsx_paths,
            report_path=report_path,
            files=file_count,
            directories=directories,
            total_bytes=total_bytes,
            filtered=filtered,
            filtered_hidden=filtered_hidden,
            filtered_system=filtered_system,
            filtered_exts=filtered_exts,
            filtered_os_noise=filtered_os_noise,
            elapsed=time.time() - started,
            hash_algorithm=normalize_hash_algorithm(hash_algorithm),
            create_xlsx=create_xlsx,
            preserve_zeros=preserve_zeros,
            delete_csv=delete_csv,
            csv_deleted=csv_deleted,
            max_rows_per_csv=max_rows_per_csv,
            csv_parts=len(csv_paths or [output_path]),
            xlsx_parts=len(xlsx_paths),
            hash_workers=hash_workers,
            top_by_count=summarize_entries(summaries, "count"),
            top_by_size=summarize_entries(summaries, "bytes"),
            first_csv_item=first_csv_item,
            last_csv_item=last_csv_item,
            agency_template=agency_template,
        )
        write_scan_report(result)
        return result
    except ScanCanceled:
        cleanup_paths = list(csv_paths)
        cleanup_paths.extend(xlsx_paths)
        cleanup_paths.extend(
            [
                output_path,
                output_path.with_suffix(".xlsx"),
                report_path,
            ]
        )
        for path in cleanup_paths:
            try:
                path.unlink()
            except FileNotFoundError:
                pass
        raise


def write_scan_report(result: ScanResult) -> None:
    result.report_path.write_text(build_scan_report(result), encoding="utf-8")


def summarize_output_parts(paths: list[Path]) -> str:
    if not paths:
        return "none"
    max_shown = 4
    labels = [path.name for path in paths[:max_shown]]
    if len(paths) > max_shown:
        return f"{', '.join(labels)} (+{len(paths) - max_shown} more)"
    return ", ".join(labels)


def build_scan_report(result: ScanResult) -> str:
    if result.folders_only:
        lines = [
            "Folder List Report",
            f"Selected folder: {result.source_name}",
            f"Saved folder list: {result.output_path.name}",
            f"Summary report: {result.report_path.name}",
            f"Folders in CSV: {result.directories}",
            f"First folder in CSV: {result.first_csv_item or 'none'}",
            f"Last folder in CSV: {result.last_csv_item or 'none'}",
            f"Finished in: {result.elapsed:.2f}s",
        ]
        return "\n".join(lines) + "\n"
    lines = [
        "Content List Report",
        f"Selected folder: {result.source_name}",
        f"Saved file list: {result.output_path.name}",
        f"CSV files created: {result.csv_parts}",
        f"Rows per CSV max: {result.max_rows_per_csv}",
        f"CSV parts: {summarize_output_parts(result.output_paths)}",
        f"Excel copy: {result.xlsx_path.name if result.xlsx_path else 'not created'}",
        f"XLSX files created: {result.xlsx_parts}",
        f"XLSX parts: {summarize_output_parts(result.xlsx_paths)}",
        f"Summary report: {result.report_path.name}",
        f"Agency template: {'on' if result.agency_template else 'off'}",
        f"Files included: {result.files}",
        f"Folders counted: {result.directories}",
        f"Total size: {human_bytes(result.total_bytes)}",
        f"Items skipped: {result.filtered}",
        f"OS noise excluded: {result.filtered_os_noise}",
        f"Verification hash: {hash_algorithm_label(result.hash_algorithm)}",
        f"First file in CSV: {result.first_csv_item or 'none'}",
        f"Last file in CSV: {result.last_csv_item or 'none'}",
        f"Delete CSV after XLSX: {'on' if result.delete_csv and result.create_xlsx else 'off'}",
        f"CSV removed after XLSX: {'on' if result.csv_deleted else 'off'}",
        f"Finished in: {result.elapsed:.2f}s",
        "",
        *render_top("Top extensions by file count", result.top_by_count),
        "",
        *render_top("Top extensions by total size", result.top_by_size, by_size=True),
    ]
    return "\n".join(lines) + "\n"


def _compute_verdict(
    missing_no_match: int,
    extra_no_match: int,
    hash_mismatches: int,
    size_mismatches: int,
    moved_files: int,
    duplicates_on_a: int,
    duplicates_on_b: int,
    metadata_only_diffs: int = 0,
) -> str:
    if missing_no_match > 0 or extra_no_match > 0 or hash_mismatches > 0 or size_mismatches > 0:
        return CLONE_VERDICT_NOT
    if metadata_only_diffs > 0:
        return CLONE_VERDICT_METADATA
    if moved_files > 0 or duplicates_on_a > 0 or duplicates_on_b > 0:
        return CLONE_VERDICT_CONTENT
    return CLONE_VERDICT_EXACT


def compare_scan_outputs(
    drive_a: ScanResult,
    drive_b: ScanResult,
    diff_path: Path,
    report_path: Path,
    soft_compare: bool = False,
    progress_callback: Callable[[CloneCompareProgress], None] | None = None,
    cancel_event=None,
) -> CloneVerificationResult:
    started = time.time()
    compared = 0
    differences = 0
    moved_files = 0
    duplicates_on_b = 0
    duplicates_on_a = 0
    missing_no_match = 0
    extra_no_match = 0
    size_mismatches = 0
    hash_mismatches = 0
    metadata_only_diffs = 0
    diff_path.parent.mkdir(parents=True, exist_ok=True)

    iterator_a = iter_scan_csv_rows(drive_a.output_paths)
    iterator_b = iter_scan_csv_rows(drive_b.output_paths)

    def safe_next(it):
        try:
            return next(it)
        except StopIteration:
            return None

    def send_progress(current_name: str) -> None:
        if progress_callback is None:
            return
        progress_callback(CloneCompareProgress(
            compared=compared,
            total=max(drive_a.files, drive_b.files),
            differences=differences,
            current_name=current_name,
        ))

    next_a = safe_next(iterator_a)
    next_b = safe_next(iterator_b)

    # unmatched_a / unmatched_b: hash → list[ScanCSVRow] for path-only rows
    unmatched_a: dict[str, list] = {}
    unmatched_b: dict[str, list] = {}
    # soft_b_index: filename::size → list[ScanCSVRow] for PDF soft compare
    soft_b_index: dict[str, list] = {}

    try:
        with diff_path.open("w", newline="", encoding="utf-8") as handle:
            writer = csv.writer(handle)
            writer.writerow(CLONE_DIFF_HEADERS)

            # ── Pass 1: streaming sorted merge ───────────────────────────────
            while next_a is not None or next_b is not None:
                if cancel_event is not None and cancel_event.is_set():
                    raise ScanCanceled()

                if (next_a is not None and next_b is not None
                        and next_a.relative_path == next_b.relative_path):
                    compared += 1
                    size_mismatch = next_a.size != next_b.size
                    hash_mismatch = (
                        next_a.hash_value != next_b.hash_value
                        or next_a.hash_algorithm != next_b.hash_algorithm
                    )
                    diff_type = ""
                    if size_mismatch and hash_mismatch:
                        diff_type = "size and hash mismatch"
                        size_mismatches += 1
                        hash_mismatches += 1
                    elif size_mismatch:
                        diff_type = "size mismatch"
                        size_mismatches += 1
                    elif hash_mismatch:
                        diff_type = "hash mismatch"
                        hash_mismatches += 1
                    if diff_type:
                        differences += 1
                        writer.writerow([
                            diff_type,
                            next_a.relative_path, next_a.file_name,
                            next_b.relative_path, next_b.file_name,
                            str(next_a.size), str(next_b.size),
                            next_a.hash_algorithm, next_b.hash_algorithm,
                            next_a.hash_value, next_b.hash_value,
                        ])
                    send_progress(next_a.relative_path)
                    next_a = safe_next(iterator_a)
                    next_b = safe_next(iterator_b)

                elif next_b is None or (next_a is not None
                        and next_a.relative_path < next_b.relative_path):
                    unmatched_a.setdefault(next_a.hash_value, []).append(next_a)
                    send_progress(next_a.relative_path)
                    next_a = safe_next(iterator_a)

                else:
                    unmatched_b.setdefault(next_b.hash_value, []).append(next_b)
                    if soft_compare and next_b.file_name.lower().endswith(".pdf"):
                        key = _soft_index_key(next_b.file_name, next_b.size)
                        soft_b_index.setdefault(key, []).append(next_b)
                    send_progress(next_b.relative_path)
                    next_b = safe_next(iterator_b)

            # ── Pass 2: in-memory hash cross-reference ────────────────────────
            for hash_val, a_rows in unmatched_a.items():
                b_rows = unmatched_b.pop(hash_val, [])
                if not b_rows:
                    for r in a_rows:
                        soft_matched = False
                        if (soft_compare and drive_a.source_dir and drive_b.source_dir
                                and r.file_name.lower().endswith(".pdf")):
                            key = _soft_index_key(r.file_name, r.size)
                            candidates = soft_b_index.get(key)
                            if candidates:
                                b_match = candidates[0]
                                path_a = drive_a.source_dir / Path(r.relative_path)
                                path_b = drive_b.source_dir / Path(b_match.relative_path)
                                if _pdf_soft_match(path_a, path_b):
                                    differences += 1
                                    metadata_only_diffs += 1
                                    writer.writerow([
                                        "metadata-only (PDF document IDs)",
                                        r.relative_path, r.file_name,
                                        b_match.relative_path, b_match.file_name,
                                        str(r.size), str(b_match.size),
                                        r.hash_algorithm, b_match.hash_algorithm,
                                        r.hash_value, b_match.hash_value,
                                    ])
                                    # remove consumed B row from both indices
                                    soft_b_index[key] = candidates[1:]
                                    if not soft_b_index[key]:
                                        del soft_b_index[key]
                                    b_hash = b_match.hash_value
                                    remaining = unmatched_b.get(b_hash, [])
                                    for i, rb in enumerate(remaining):
                                        if rb.relative_path == b_match.relative_path:
                                            unmatched_b[b_hash] = remaining[:i] + remaining[i+1:]
                                            break
                                    if not unmatched_b.get(b_hash):
                                        unmatched_b.pop(b_hash, None)
                                    soft_matched = True
                        if not soft_matched:
                            differences += 1
                            missing_no_match += 1
                            writer.writerow([
                                "missing from 2nd Drive (no match)",
                                r.relative_path, r.file_name,
                                "", "",
                                str(r.size), "",
                                r.hash_algorithm, "",
                                r.hash_value, "",
                            ])
                    continue

                match_count = min(len(a_rows), len(b_rows))
                for i in range(match_count):
                    a, b = a_rows[i], b_rows[i]
                    differences += 1
                    moved_files += 1
                    writer.writerow([
                        "moved/renamed",
                        a.relative_path, a.file_name,
                        b.relative_path, b.file_name,
                        str(a.size), str(b.size),
                        a.hash_algorithm, b.hash_algorithm,
                        a.hash_value, b.hash_value,
                    ])
                for b in b_rows[match_count:]:
                    differences += 1
                    duplicates_on_b += 1
                    writer.writerow([
                        "duplicate on 2nd Drive",
                        "", "",
                        b.relative_path, b.file_name,
                        "", str(b.size),
                        "", b.hash_algorithm,
                        "", b.hash_value,
                    ])
                for a in a_rows[match_count:]:
                    differences += 1
                    duplicates_on_a += 1
                    writer.writerow([
                        "duplicate on 1st Drive",
                        a.relative_path, a.file_name,
                        "", "",
                        str(a.size), "",
                        a.hash_algorithm, "",
                        a.hash_value, "",
                    ])

            for b_rows in unmatched_b.values():
                for b in b_rows:
                    differences += 1
                    extra_no_match += 1
                    writer.writerow([
                        "extra on 2nd Drive (no match)",
                        "", "",
                        b.relative_path, b.file_name,
                        "", str(b.size),
                        "", b.hash_algorithm,
                        "", b.hash_value,
                    ])

    except Exception:
        diff_path.unlink(missing_ok=True)
        report_path.unlink(missing_ok=True)
        raise

    verdict = _compute_verdict(
        missing_no_match, extra_no_match, hash_mismatches, size_mismatches,
        moved_files, duplicates_on_a, duplicates_on_b, metadata_only_diffs,
    )
    result = CloneVerificationResult(
        drive_a=drive_a,
        drive_b=drive_b,
        diff_path=diff_path,
        report_path=report_path,
        hash_algorithm=normalize_hash_algorithm(drive_a.hash_algorithm),
        elapsed=time.time() - started,
        verdict=verdict,
        compared=compared,
        differences=differences,
        moved_files=moved_files,
        duplicates_on_b=duplicates_on_b,
        duplicates_on_a=duplicates_on_a,
        missing_no_match=missing_no_match,
        extra_no_match=extra_no_match,
        size_mismatches=size_mismatches,
        hash_mismatches=hash_mismatches,
        excluded_system=drive_a.filtered_os_noise + drive_b.filtered_os_noise,
        metadata_only_diffs=metadata_only_diffs,
        soft_compare=soft_compare,
    )
    write_clone_verification_report(result)
    return result


def write_clone_verification_report(result: CloneVerificationResult) -> None:
    result.report_path.write_text(build_clone_verification_report(result), encoding="utf-8")


def _verdict_summary_lines(result: CloneVerificationResult) -> list[str]:
    if result.verdict == CLONE_VERDICT_EXACT:
        return [
            "EXACT CLONE — All files verified present on both drives at identical paths.",
            "No files are missing, moved, or corrupted.",
        ]
    if result.verdict == CLONE_VERDICT_METADATA:
        lines = [
            "METADATA CLONE — All file content verified present on both drives.",
            f"{result.metadata_only_diffs} PDF file(s) differ only in embedded document IDs (export metadata), not content.",
            "No files are missing or corrupted. Both drives were independently exported from the same source.",
        ]
        if result.moved_files:
            lines.append(f"{result.moved_files} file(s) also detected at different paths (folder renamed or moved).")
        return lines
    if result.verdict == CLONE_VERDICT_CONTENT:
        lines = [
            "CONTENT CLONE — All files verified present on both drives.",
            f"{result.moved_files} file(s) detected at different paths (folder renamed or moved).",
            "No files are missing or corrupted.",
        ]
        if result.duplicates_on_b:
            lines.append(f"{result.duplicates_on_b} extra duplicate(s) found on 2nd Drive.")
        if result.duplicates_on_a:
            lines.append(f"{result.duplicates_on_a} extra duplicate(s) found on 1st Drive.")
        return lines
    lines = ["NOT A CLONE — Verification failed."]
    if result.missing_no_match:
        lines.append(f"{result.missing_no_match} file(s) missing from 2nd Drive with no hash match anywhere.")
    if result.extra_no_match:
        lines.append(f"{result.extra_no_match} extra file(s) on 2nd Drive with no hash match anywhere.")
    if result.hash_mismatches:
        lines.append(f"{result.hash_mismatches} file(s) at matching paths have different hash values (possible corruption).")
    return lines


def build_clone_verification_report(result: CloneVerificationResult) -> str:
    lines = [
        "Clone Verification Report",
        f"Verdict: {result.verdict}",
        "━" * 38,
        f"1st Drive folder: {result.drive_a.source_name}",
        f"2nd Drive folder: {result.drive_b.source_name}",
        f"1st Drive content list: {result.drive_a.output_path.name}",
        f"2nd Drive content list: {result.drive_b.output_path.name}",
        f"1st Drive summary report: {result.drive_a.report_path.name}",
        f"2nd Drive summary report: {result.drive_b.report_path.name}",
        f"Differences CSV: {result.diff_path.name}",
        f"Verification hash: {hash_algorithm_label(result.hash_algorithm)}",
        "",
        f"Exact path + content matches: {result.compared}",
        f"Content matches (moved/renamed): {result.moved_files}",
        f"Metadata-only matches (PDF document IDs): {result.metadata_only_diffs}",
        f"Hash mismatches: {result.hash_mismatches}",
        "",
        f"⚠ Missing from 2nd Drive (no hash match): {result.missing_no_match}",
        f"⚠ Extra on 2nd Drive (no hash match): {result.extra_no_match}",
        "",
        f"Duplicates on 2nd Drive: {result.duplicates_on_b}",
        f"Duplicates on 1st Drive: {result.duplicates_on_a}",
        f"System paths excluded: {result.excluded_system}",
        f"Finished in: {result.elapsed:.2f}s",
        "",
    ]
    lines.extend(_verdict_summary_lines(result))
    return "\n".join(lines) + "\n"


def build_clone_verification_summary(result: CloneVerificationResult) -> str:
    lines = [
        f"Verdict: {result.verdict}",
        f"1st Drive folder: {result.drive_a.source_name}",
        f"2nd Drive folder: {result.drive_b.source_name}",
        f"Differences CSV: {result.diff_path.name}",
        f"Clone report: {result.report_path.name}",
        f"Verification hash: {hash_algorithm_label(result.hash_algorithm)}",
        f"Exact matches: {result.compared}",
        f"Moved/renamed: {result.moved_files}",
        f"Hash mismatches: {result.hash_mismatches}",
        f"Missing (no match): {result.missing_no_match}",
        f"Extra (no match): {result.extra_no_match}",
        f"Finished in: {result.elapsed:.2f}s",
    ]
    return "\n".join(lines)


def delete_deferred_scan_csvs(result: ScanResult, delete_requested: bool) -> None:
    result.delete_csv = delete_requested
    if not delete_requested or not result.create_xlsx:
        return
    for csv_path in result.output_paths:
        csv_path.unlink(missing_ok=True)
    result.csv_deleted = True
    write_scan_report(result)


def collect_email_matches(source_dir: Path) -> list[Path]:
    matches = [path for path in source_dir.rglob("*") if path.is_file() and path.suffix.lower() in EMAIL_EXTENSIONS]
    matches.sort()
    return matches


def is_path_within(candidate: Path, root: Path) -> bool:
    try:
        candidate.relative_to(root)
        return candidate != root
    except ValueError:
        return False


def copy_email_files(
    source_dir: Path,
    dest_dir: Path,
    progress_callback: Callable[[EmailCopyProgress], None] | None = None,
) -> EmailCopyResult:
    if not source_dir.is_dir():
        raise ValueError(f"Source folder does not exist: {source_dir}")
    source_dir = source_dir.resolve()
    dest_dir = dest_dir.resolve()
    if source_dir == dest_dir:
        raise ValueError("Destination folder must be different from the source folder.")
    if is_path_within(dest_dir, source_dir):
        raise ValueError("Destination folder cannot be inside the source folder.")

    started = time.time()
    dest_dir.mkdir(parents=True, exist_ok=True)
    manifest_path = dest_dir / default_manifest_name()
    matches: list[Path] = []
    scanned = 0
    for path in sorted(source_dir.rglob("*")):
        if not path.is_file():
            continue
        scanned += 1
        if progress_callback is not None:
            progress_callback(
                EmailCopyProgress(
                    phase="scanning",
                    scanned=scanned,
                    matched=len(matches),
                    copied=0,
                    total=0,
                    current_name=path.name,
                )
            )
        if path.suffix.lower() in EMAIL_EXTENSIONS:
            matches.append(path)
            if progress_callback is not None:
                progress_callback(
                    EmailCopyProgress(
                        phase="scanning",
                        scanned=scanned,
                        matched=len(matches),
                        copied=0,
                        total=0,
                        current_name=path.name,
                    )
                )
    if progress_callback is not None:
        progress_callback(
            EmailCopyProgress(
                phase="copying",
                scanned=scanned,
                matched=len(matches),
                copied=0,
                total=len(matches),
            )
        )
    copied = 0

    with manifest_path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(EMAIL_MANIFEST_HEADERS)

        for path in matches:
            relative = path.relative_to(source_dir)
            target = dest_dir / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(path, target)
            writer.writerow(
                [
                    str(path),
                    str(target),
                    relative.as_posix(),
                    path.name,
                    path.suffix.lower(),
                    str(path.stat().st_size),
                ]
            )
            copied += 1
            if progress_callback is not None:
                progress_callback(
                    EmailCopyProgress(
                        phase="copying",
                        scanned=scanned,
                        matched=len(matches),
                        copied=copied,
                        total=len(matches),
                        current_relative=relative.as_posix(),
                        current_name=path.name,
                    )
                )

    return EmailCopyResult(
        source_dir=source_dir,
        dest_dir=dest_dir,
        manifest_path=manifest_path,
        copied=copied,
        elapsed=time.time() - started,
    )


def build_scan_summary(result: ScanResult) -> str:
    if result.folders_only:
        lines = [
            "Your folder list is ready.",
            f"Selected folder: {result.source_name}",
            f"Saved folder list: {result.output_path.name}",
            f"Folders in CSV: {result.directories}",
            f"First folder in CSV: {result.first_csv_item or 'none'}",
            f"Last folder in CSV: {result.last_csv_item or 'none'}",
            f"Finished in: {result.elapsed:.2f}s",
        ]
        return "\n".join(lines)
    lines = [
        "Your file list is ready.",
        f"Selected folder: {result.source_name}",
        f"Saved file list: {result.output_path.name}",
        f"CSV files created: {result.csv_parts}",
        f"Rows per CSV max: {result.max_rows_per_csv}",
        f"CSV parts: {summarize_output_parts(result.output_paths)}",
        f"Excel copy: {result.xlsx_path.name if result.xlsx_path else 'not created'}",
        f"XLSX files created: {result.xlsx_parts}",
        f"XLSX parts: {summarize_output_parts(result.xlsx_paths)}",
        f"Summary report: {result.report_path.name}",
        f"Agency template: {'on' if result.agency_template else 'off'}",
        f"Files included: {result.files}",
        f"Folders counted: {result.directories}",
        f"Total size: {result.total_bytes} ({human_bytes(result.total_bytes)})",
        f"Items skipped: {result.filtered}",
        f"Hidden items skipped: {result.filtered_hidden}",
        f"System items skipped: {result.filtered_system}",
        f"Skipped by file type: {result.filtered_exts}",
        f"Verification hash: {hash_algorithm_label(result.hash_algorithm)}",
        f"First file in CSV: {result.first_csv_item or 'none'}",
        f"Last file in CSV: {result.last_csv_item or 'none'}",
        f"Excel copy enabled: {'on' if result.create_xlsx else 'off'}",
        f"Keep leading zeros in Excel: {'on' if result.preserve_zeros and result.create_xlsx else 'off'}",
        f"Delete CSV after XLSX: {'on' if result.delete_csv and result.create_xlsx else 'off'}",
        f"CSV removed after XLSX: {'on' if result.csv_deleted else 'off'}",
        f"Finished in: {result.elapsed:.2f}s",
        "",
        *render_top("Most common file types", result.top_by_count),
        "",
        *render_top("Largest file types by size", result.top_by_size, by_size=True),
    ]
    return "\n".join(lines)
