#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import os
import queue
import subprocess
import sys
import threading
import webbrowser
from pathlib import Path

try:
    import tkinter as tk
    from tkinter import filedialog, messagebox, ttk
except Exception:  # pragma: no cover
    tk = None
    filedialog = None
    messagebox = None
    ttk = None

try:
    import customtkinter as ctk
except Exception:  # pragma: no cover
    ctk = None

from content_list_core import (
    CloneCompareProgress,
    CloneVerificationResult,
    DEFAULT_MAX_ROWS_PER_CSV,
    EMAIL_EXTENSIONS,
    EmailCopyProgress,
    EmailCopyResult,
    HASH_ALGORITHM_BLAKE3,
    HASH_ALGORITHM_SHA1,
    HASH_ALGORITHM_SHA256,
    ScanCanceled,
    ScanProgress,
    build_clone_verification_summary,
    build_scan_summary,
    clone_diff_csv_path,
    clone_diff_report_path,
    clone_output_path_for_drive_b,
    compare_progress_fraction,
    compare_scan_outputs,
    copy_email_files,
    csv_output_path_for_part,
    delete_deferred_scan_csvs,
    default_folder_list_output_name,
    default_output_name,
    default_hash_algorithm,
    hash_algorithm_label,
    hash_algorithm_labels,
    human_bytes,
    is_blake3_available,
    normalize_agency_template_fields,
    normalize_exts,
    normalize_hash_algorithm,
    run_scan,
)
from deps_check import check_deps, format_banner_text


PLACEHOLDER_GITHUB_URL = "https://github.com/placeholder/content-list-generator"
SETTINGS_FILE_NAME = "content-list-generator-settings.json"
SETTINGS_ENV_VAR = "CONTENT_LIST_GENERATOR_SETTINGS"
LEGACY_SETTINGS_PATH = Path.home() / ".content-list-generator-settings.json"
THEME_MODE_SYSTEM = "system"
THEME_MODE_LIGHT = "light"
THEME_MODE_DARK = "dark"
THEME_MODE_LABELS = {
    THEME_MODE_SYSTEM: "System",
    THEME_MODE_LIGHT: "Light",
    THEME_MODE_DARK: "Dark",
}

def default_settings_path() -> Path:
    return Path.home() / "scripts" / "settings" / SETTINGS_FILE_NAME


def read_json_dict(path: Path) -> dict[str, object]:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError, TypeError):
        return {}
    if not isinstance(data, dict):
        return {}
    return data


def write_json_dict(path: Path, payload: dict[str, object]) -> None:
    try:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(payload, indent=2), encoding="utf-8")
    except OSError:
        return


def resolve_env_settings_path(raw_path: str) -> Path:
    candidate = Path(raw_path).expanduser()
    if candidate.suffix:
        return candidate
    return candidate / SETTINGS_FILE_NAME


def current_settings_path() -> Path:
    env_path = os.environ.get(SETTINGS_ENV_VAR, "").strip()
    if env_path:
        return resolve_env_settings_path(env_path)
    return default_settings_path()


def normalize_theme_mode(value: str) -> str:
    mode = str(value or "").strip().lower()
    if mode in {THEME_MODE_SYSTEM, THEME_MODE_LIGHT, THEME_MODE_DARK}:
        return mode
    return THEME_MODE_SYSTEM


def theme_mode_label(value: str) -> str:
    return THEME_MODE_LABELS[normalize_theme_mode(value)]


def theme_mode_from_label(value: str) -> str:
    mode = str(value or "").strip().lower()
    for key, label in THEME_MODE_LABELS.items():
        if mode == label.lower():
            return key
    return normalize_theme_mode(mode)


def effective_theme_mode(preferred_mode: str) -> str:
    preferred = normalize_theme_mode(preferred_mode)
    if preferred != THEME_MODE_SYSTEM:
        return preferred
    if ctk is None:
        return THEME_MODE_LIGHT
    resolved = str(ctk.get_appearance_mode() or "").strip().lower()
    if resolved == THEME_MODE_DARK:
        return THEME_MODE_DARK
    return THEME_MODE_LIGHT


def load_theme_mode() -> str:
    for candidate in (current_settings_path(), LEGACY_SETTINGS_PATH, default_settings_path()):
        data = read_json_dict(candidate)
        mode = normalize_theme_mode(str(data.get("appearance_mode", "")))
        if mode in {THEME_MODE_SYSTEM, THEME_MODE_DARK, THEME_MODE_LIGHT}:
            return mode
    return THEME_MODE_SYSTEM


def save_theme_mode(mode: str) -> None:
    data = read_json_dict(current_settings_path())
    data["appearance_mode"] = normalize_theme_mode(mode)
    write_json_dict(current_settings_path(), data)


def palette_for_mode(mode: str) -> dict[str, str]:
    if mode == "dark":
        return {
            "app_bg": "#0c131b",
            "sidebar_bg": "#101923",
            "hero_bg": "#0c1722",
            "hero_card_bg": "#122130",
            "card_bg": "#13202b",
            "card_alt_bg": "#182633",
            "title_fg": "#edf4fb",
            "hero_fg": "#f5f9fd",
            "hero_muted": "#bfd0df",
            "body_fg": "#ebf1f7",
            "hint_fg": "#9caebf",
            "entry_bg": "#0f1822",
            "entry_fg": "#edf4fb",
            "border": "#263749",
            "progress_trough": "#243443",
            "progress_fill": "#5e9fff",
            "primary_bg": "#4d8ef8",
            "primary_hover": "#69a2ff",
            "primary_fg": "#f8fbff",
            "secondary_bg": "#1b2a38",
            "secondary_hover": "#213447",
            "secondary_fg": "#edf4fb",
            "selection_bg": "#204a73",
            "selection_fg": "#f8fbff",
            "sidebar_active_bg": "#173455",
            "sidebar_active_fg": "#f6fbff",
            "sidebar_idle_fg": "#b4c4d3",
            "chip_bg": "#0f1822",
            "success_fg": "#9fd2a2",
        }
    return {
        "app_bg": "#ebf1f5",
        "sidebar_bg": "#f2f4f6",
        "hero_bg": "#13324a",
        "hero_card_bg": "#f7fafc",
        "card_bg": "#ffffff",
        "card_alt_bg": "#f3f6f9",
        "title_fg": "#14324a",
        "hero_fg": "#ffffff",
        "hero_muted": "#dce8f3",
        "body_fg": "#243849",
        "hint_fg": "#556778",
        "entry_bg": "#ffffff",
        "entry_fg": "#243746",
        "border": "#cad6e0",
        "progress_trough": "#d7e1ea",
        "progress_fill": "#005bc1",
        "primary_bg": "#005bc1",
        "primary_hover": "#0070eb",
        "primary_fg": "#ffffff",
        "secondary_bg": "#eef3f7",
        "secondary_hover": "#e1e8ef",
        "secondary_fg": "#32485a",
        "selection_bg": "#d7e7ff",
        "selection_fg": "#12324a",
        "sidebar_active_bg": "#d6e6ff",
        "sidebar_active_fg": "#0f4c98",
        "sidebar_idle_fg": "#526276",
        "chip_bg": "#eef3f7",
        "success_fg": "#2d7c48",
    }


LIGHT_PALETTE = palette_for_mode("light")
DARK_PALETTE = palette_for_mode("dark")


def themed_color(key: str) -> tuple[str, str]:
    return (LIGHT_PALETTE[key], DARK_PALETTE[key])


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Recursive content list generator")
    parser.add_argument("--mode", choices=("scan", "email-copy"), default="scan")
    parser.add_argument("--source")
    parser.add_argument("--output-dir")
    parser.add_argument("--output-name")
    parser.add_argument("--dest")
    parser.add_argument("--hash", action="store_true", dest="hashing", help="Use SHA-256 hashing for backward-compatible CLI usage")
    parser.add_argument("--hash-algorithm", choices=("off", "blake3", "sha1", "sha256"))
    parser.add_argument("--skip-hidden", action=argparse.BooleanOptionalAction, default=True)
    parser.add_argument("--skip-system", action=argparse.BooleanOptionalAction, default=True)
    parser.add_argument("--include-hidden", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument("--include-system", action="store_true", help=argparse.SUPPRESS)
    parser.add_argument("--exclude-exts", default="")
    parser.add_argument("--overwrite", action="store_true")
    parser.add_argument("--xlsx", action=argparse.BooleanOptionalAction, dest="create_xlsx", default=True)
    parser.add_argument("--preserve-zeros", action=argparse.BooleanOptionalAction, default=True)
    parser.add_argument("--delete-csv-after-xlsx", action=argparse.BooleanOptionalAction, default=True)
    parser.add_argument("--max-rows-per-csv", type=int, default=DEFAULT_MAX_ROWS_PER_CSV)
    parser.add_argument("--agency-template", action="store_true", help="Write agency content-list headers instead of the standard scan headers")
    parser.add_argument("--agency-rg", default="")
    parser.add_argument("--agency-sg", default="")
    parser.add_argument("--agency-series", default="")
    parser.add_argument("--agency-rc-series", default="")
    parser.add_argument("--agency-dept-organization", default="")
    parser.add_argument("--agency-division", default="")
    parser.add_argument("--agency-section", default="")
    parser.add_argument("--agency-unit", default="")
    parser.add_argument("--agency-rc-series-name", default="")
    parser.add_argument("--agency-begin-date", default="")
    parser.add_argument("--agency-end-date", default="")
    parser.add_argument("--agency-description", default="")
    parser.add_argument("--agency-location", default="")
    parser.add_argument("--agency-material-type", default="Born Digital")
    parser.add_argument("--agency-comments", default="")
    parser.add_argument("--agency-confidential", default="")
    parser.add_argument("--agency-disposition-date", default="")
    parser.add_argument("--agency-box-num", default="")
    parser.add_argument("--agency-td-num", default="")
    parser.add_argument("--agency-location-id", default="")
    parser.add_argument("--agency-record-level", default="Item")
    parser.add_argument("--cli", action="store_true", help="Force CLI mode instead of Tkinter GUI")
    return parser.parse_args()


def prompt(text: str, default: str = "") -> str:
    if not sys.stdin.isatty():
        return default
    suffix = f" [{default}]" if default else ""
    value = input(f"{text}{suffix}: ").strip()
    return value or default


def prompt_yes_no(text: str, default: bool = False) -> bool:
    if not sys.stdin.isatty():
        return default
    hint = "Y/n" if default else "y/N"
    value = input(f"{text} [{hint}]: ").strip().lower()
    if not value:
        return default
    return value in {"y", "yes"}


def cli_flag_provided(*flags: str) -> bool:
    for arg in sys.argv[1:]:
        for flag in flags:
            if arg == flag or arg.startswith(f"{flag}="):
                return True
    return False


def open_in_file_manager(path: Path) -> None:
    target = path.expanduser().resolve()
    if sys.platform == "darwin":
        subprocess.run(["open", str(target)], check=False)
        return
    if os.name == "nt":
        os.startfile(str(target))  # type: ignore[attr-defined]
        return
    subprocess.run(["xdg-open", str(target)], check=False)


def ejectable_volume_root(path: Path) -> Path | None:
    target = path.expanduser().resolve()
    path_str = str(target)
    if sys.platform == "darwin":
        parts = target.parts
        if len(parts) >= 3 and parts[0] == "/" and parts[1] == "Volumes":
            return Path("/", "Volumes", parts[2])
        return None
    if os.name == "nt":
        drive = target.drive
        if drive:
            return Path(f"{drive}\\")
        return None
    for prefix in (Path("/media"), Path("/run/media"), Path("/mnt")):
        prefix_str = str(prefix)
        if not path_str.startswith(prefix_str + os.sep):
            continue
        relative_parts = target.relative_to(prefix).parts
        if prefix in {Path("/media"), Path("/run/media")}:
            if len(relative_parts) >= 2:
                return prefix / relative_parts[0] / relative_parts[1]
            return None
        if relative_parts:
            return prefix / relative_parts[0]
    return None


def eject_drive_or_folder(path: Path) -> tuple[bool, str]:
    root = ejectable_volume_root(path)
    if root is None:
        return False, "The selected Drive/Folder A is not on a removable volume the app can eject automatically."
    if sys.platform == "darwin":
        command = ["diskutil", "eject", str(root)]
    elif os.name == "nt":
        return False, "Automatic eject is not available in this build on Windows. Remove the 1st Drive manually, then continue to the Clone/2nd Drive."
    else:
        command = ["umount", str(root)]
    completed = subprocess.run(command, capture_output=True, text=True, check=False)
    if completed.returncode != 0:
        details = (completed.stderr or completed.stdout or "").strip()
        if details:
            return False, details
        return False, "The app could not eject Drive/Folder A automatically."
    return True, f"Ejected {root}. You can now continue to the Clone/2nd Drive."


def default_scan_output_dir(source_dir: Path) -> Path:
    source = source_dir.expanduser().resolve()
    for candidate in (source.parent, Path.home(), Path.cwd()):
        try:
            resolved = candidate.expanduser().resolve()
        except OSError:
            continue
        if resolved != source:
            return resolved
    return source


def choose_directory(parent, title: str, initialdir: str, mustexist: bool, colors: dict[str, str] | None = None) -> str:
    del colors
    if filedialog is None:
        return ""
    return filedialog.askdirectory(
        parent=parent,
        title=title,
        initialdir=initialdir or os.getcwd(),
        mustexist=mustexist,
    )


def folder_places() -> list[tuple[str, Path]]:
    places: list[tuple[str, Path]] = []

    def add_place(label: str, path: Path) -> None:
        try:
            resolved = path.expanduser().resolve()
        except Exception:
            return
        if not resolved.is_dir():
            return
        if any(existing == resolved for _, existing in places):
            return
        places.append((label, resolved))

    home = Path.home()
    add_place("Home", home)
    add_place("Desktop", home / "Desktop")
    add_place("Documents", home / "Documents")
    add_place("Downloads", home / "Downloads")
    add_place("Computer", Path("/"))

    if sys.platform == "darwin":
        volumes = Path("/Volumes")
        if volumes.is_dir():
            for entry in sorted(volumes.iterdir(), key=lambda item: item.name.lower()):
                if entry.is_dir() and not entry.name.startswith("."):
                    add_place(entry.name, entry)
    else:
        for root in (Path("/media"), Path("/run/media"), Path("/mnt")):
            if not root.is_dir():
                continue
            for entry in sorted(root.iterdir(), key=lambda item: item.name.lower()):
                if not entry.is_dir():
                    continue
                add_place(entry.name, entry)
                for sub in sorted(entry.iterdir(), key=lambda item: item.name.lower()):
                    if sub.is_dir():
                        add_place(sub.name, sub)

    return places


def folder_children(path: Path) -> list[tuple[str, Path, str]]:
    items: list[tuple[str, Path, str]] = []
    if path.parent != path:
        parent_name = path.parent.name or str(path.parent)
        items.append(("(Parent)", path.parent, f"Back to {parent_name}"))

    try:
        entries = sorted(path.iterdir(), key=lambda item: item.name.lower())
    except OSError:
        return items

    for entry in entries:
        if not entry.is_dir() or entry.name.startswith("."):
            continue
        subtitle = "Restricted access"
        try:
            visible = [child for child in entry.iterdir() if not child.name.startswith(".")]
            subtitle = f"{len(visible)} items"
        except OSError:
            pass
        items.append((entry.name, entry, subtitle))
    return items


def breadcrumb_text(path: Path) -> str:
    if sys.platform == "darwin" and path.parts[:2] == ("/", "Volumes"):
        return "Volumes  >  " + "  >  ".join(path.parts[2:])
    if path == Path("/"):
        return "/"
    return "  >  ".join(part for part in path.parts if part and part != "/")


class FolderPickerDialog:
    def __init__(self, parent, title: str, initialdir: str, mustexist: bool, colors: dict[str, str]) -> None:
        self.parent = parent
        self.title = title
        self.mustexist = mustexist
        self.colors = colors
        self.result = ""
        self.current_path = Path(initialdir or os.getcwd()).expanduser()
        if not self.current_path.exists():
            self.current_path = self.current_path.parent
        if not self.current_path.is_dir():
            self.current_path = Path.home()

        self.window = tk.Toplevel(parent)
        self.window.title("Content List Toolkit")
        self.window.geometry("1180x840")
        self.window.minsize(980, 720)
        self.window.configure(bg=self.colors["app_bg"])
        self.window.transient(parent)
        self.window.grab_set()

        self.path_var = tk.StringVar(value=str(self.current_path))
        self.footer_var = tk.StringVar(value=f"Current selected folder: {self.current_path}")

        self.place_list: tk.Listbox | None = None
        self.folder_list: tk.Listbox | None = None
        self.folder_details: list[tuple[str, Path, str]] = []
        self.places = folder_places()

        self.build_ui()
        self.refresh_lists()
        self.window.after(50, self.focus_folder_list)

    def build_ui(self) -> None:
        shell = ttk.Frame(self.window, style="App.TFrame", padding=24)
        shell.pack(fill="both", expand=True)
        shell.columnconfigure(1, weight=1)
        shell.rowconfigure(2, weight=1)

        ttk.Label(shell, text="Content List Toolkit", style="Title.TLabel").grid(row=0, column=0, columnspan=2, sticky="w")

        top_bar = ttk.Frame(shell, style="Card.TFrame", padding=18)
        top_bar.grid(row=1, column=0, columnspan=2, sticky="ew", pady=(18, 0))
        top_bar.columnconfigure(0, weight=1)
        ttk.Label(top_bar, textvariable=self.path_var, style="Body.TLabel", wraplength=760, justify="left").grid(row=0, column=0, sticky="w")
        actions = ttk.Frame(top_bar, style="Card.TFrame")
        actions.grid(row=0, column=1, sticky="e")
        ttk.Button(actions, text="Up", style="Secondary.TButton", command=self.go_up).pack(side="left")
        ttk.Button(actions, text="New Folder", style="Secondary.TButton", command=self.new_folder).pack(side="left", padx=(10, 0))

        side = ttk.Frame(shell, style="Card.TFrame", padding=18)
        side.grid(row=2, column=0, sticky="nsw", pady=(14, 0))
        side.columnconfigure(0, weight=1)
        ttk.Label(side, text="LOCATIONS", style="Body.TLabel").grid(row=0, column=0, sticky="w")
        ttk.Label(side, text="Places", style="CardHint.TLabel").grid(row=1, column=0, sticky="w", pady=(2, 14))
        self.place_list = tk.Listbox(side, activestyle="none", exportselection=False, height=18, bd=0, highlightthickness=0, font=("Segoe UI", 14))
        self.place_list.configure(
            bg=self.colors["card_bg"],
            fg=self.colors["body_fg"],
            selectbackground=self.colors["selection_bg"],
            selectforeground=self.colors["selection_fg"],
            highlightbackground=self.colors["border"],
        )
        self.place_list.grid(row=2, column=0, sticky="nsew")
        self.place_list.bind("<<ListboxSelect>>", self.on_place_select)

        main = ttk.Frame(shell, style="Card.TFrame", padding=18)
        main.grid(row=2, column=1, sticky="nsew", padx=(14, 0), pady=(14, 0))
        main.columnconfigure(0, weight=1)
        main.rowconfigure(0, weight=1)
        self.folder_list = tk.Listbox(main, activestyle="none", exportselection=False, bd=0, highlightthickness=0, font=("Segoe UI", 15), selectmode="browse")
        self.folder_list.configure(
            bg=self.colors["card_bg"],
            fg=self.colors["body_fg"],
            selectbackground=self.colors["selection_bg"],
            selectforeground=self.colors["selection_fg"],
            highlightbackground=self.colors["border"],
        )
        self.folder_list.grid(row=0, column=0, sticky="nsew")
        self.folder_list.bind("<<ListboxSelect>>", self.on_folder_select)
        self.folder_list.bind("<Double-Button-1>", self.on_folder_activate)

        footer = ttk.Frame(shell, style="Card.TFrame", padding=18)
        footer.grid(row=3, column=0, columnspan=2, sticky="ew", pady=(14, 0))
        footer.columnconfigure(0, weight=1)
        ttk.Label(footer, textvariable=self.footer_var, style="Body.TLabel", wraplength=900, justify="left").grid(row=0, column=0, sticky="w")
        footer_actions = ttk.Frame(footer, style="Card.TFrame")
        footer_actions.grid(row=0, column=1, sticky="e")
        ttk.Button(footer_actions, text="Cancel", style="Secondary.TButton", command=self.cancel).pack(side="left")
        ttk.Button(footer_actions, text="Open", style="Primary.TButton", command=self.open).pack(side="left", padx=(10, 0))

    def focus_folder_list(self) -> None:
        if self.folder_list is not None and self.folder_list.winfo_exists():
            self.folder_list.focus_force()

    def refresh_lists(self) -> None:
        self.path_var.set(breadcrumb_text(self.current_path))
        self.footer_var.set(f"Current selected folder: {self.current_path}")

        if self.place_list is not None:
            self.place_list.delete(0, "end")
            for label, _ in self.places:
                self.place_list.insert("end", label)
            best_index = -1
            best_length = -1
            for index, (_, path) in enumerate(self.places):
                try:
                    self.current_path.relative_to(path)
                    if len(str(path)) > best_length:
                        best_index = index
                        best_length = len(str(path))
                except ValueError:
                    continue
            if best_index >= 0:
                self.place_list.selection_clear(0, "end")
                self.place_list.selection_set(best_index)

        self.folder_details = folder_children(self.current_path)
        if self.folder_list is not None:
            self.folder_list.delete(0, "end")
            for name, _, subtitle in self.folder_details:
                self.folder_list.insert("end", f"{name}\n   {subtitle}")

    def on_place_select(self, _event=None) -> None:
        if self.place_list is None:
            return
        selection = self.place_list.curselection()
        if not selection:
            return
        _, path = self.places[selection[0]]
        self.current_path = path
        self.refresh_lists()

    def on_folder_select(self, _event=None) -> None:
        if self.folder_list is None:
            return
        selection = self.folder_list.curselection()
        if not selection:
            self.footer_var.set(f"Current selected folder: {self.current_path}")
            return
        _, path, _ = self.folder_details[selection[0]]
        self.footer_var.set(f"Current selected folder: {path}")

    def on_folder_activate(self, _event=None) -> None:
        if self.folder_list is None:
            return
        selection = self.folder_list.curselection()
        if not selection:
            return
        _, path, _ = self.folder_details[selection[0]]
        self.current_path = path
        self.refresh_lists()

    def go_up(self) -> None:
        if self.current_path.parent != self.current_path:
            self.current_path = self.current_path.parent
            self.refresh_lists()

    def new_folder(self) -> None:
        entry_window = tk.Toplevel(self.window)
        entry_window.title("New Folder")
        entry_window.transient(self.window)
        entry_window.grab_set()
        entry_window.geometry("420x170")
        entry_window.minsize(360, 150)
        entry_window.configure(bg=self.colors["app_bg"])

        frame = ttk.Frame(entry_window, style="App.TFrame", padding=20)
        frame.pack(fill="both", expand=True)
        ttk.Label(frame, text="Create a folder inside:", style="AppBody.TLabel").pack(anchor="w")
        ttk.Label(frame, text=str(self.current_path), style="Hint.TLabel", wraplength=360, justify="left").pack(anchor="w", pady=(4, 12))
        ttk.Label(frame, text="Folder name", style="AppBody.TLabel").pack(anchor="w")
        entry = ttk.Entry(frame, style="App.TEntry")
        entry.pack(fill="x", pady=(6, 14))

        def create_folder() -> None:
            name = entry.get().strip()
            if not name:
                messagebox.showerror("Folder name required", "Enter a folder name.", parent=entry_window)
                return
            target = self.current_path / name
            try:
                target.mkdir(parents=True, exist_ok=True)
            except OSError as exc:
                messagebox.showerror("Create folder failed", str(exc), parent=entry_window)
                return
            self.current_path = target
            self.refresh_lists()
            entry_window.destroy()

        buttons = ttk.Frame(frame, style="App.TFrame")
        buttons.pack(anchor="e")
        ttk.Button(buttons, text="Cancel", style="Secondary.TButton", command=entry_window.destroy).pack(side="left")
        ttk.Button(buttons, text="Create Folder", style="Primary.TButton", command=create_folder).pack(side="left", padx=(10, 0))

        entry_window.after(50, entry.focus_force)

    def open(self) -> None:
        selected = self.current_path
        if self.folder_list is not None:
            selection = self.folder_list.curselection()
            if selection:
                _, selected, _ = self.folder_details[selection[0]]
        self.result = str(selected)
        self.window.destroy()

    def cancel(self) -> None:
        self.result = ""
        self.window.destroy()

    def show(self) -> str:
        self.window.wait_window()
        return self.result

def run_cli_scan(args: argparse.Namespace) -> int:
    source_dir = Path(args.source or prompt("Source folder", os.getcwd())).expanduser().resolve()
    if not source_dir.is_dir():
        print(f"Source folder does not exist: {source_dir}", file=sys.stderr)
        return 1

    output_dir = Path(args.output_dir or prompt("Output folder", str(default_scan_output_dir(source_dir)))).expanduser().resolve()
    if output_dir == source_dir:
        print("Output folder cannot be the same as the source folder.", file=sys.stderr)
        return 1
    output_dir.mkdir(parents=True, exist_ok=True)

    output_name = args.output_name or prompt("Output file name", default_output_name(source_dir))
    if not output_name.lower().endswith(".csv"):
        print("Output file name must end in .csv", file=sys.stderr)
        return 1

    hash_algorithm = normalize_hash_algorithm(args.hash_algorithm)
    if args.hashing and not args.hash_algorithm:
        hash_algorithm = HASH_ALGORITHM_SHA256
    if args.hash_algorithm is None and not args.hashing:
        hash_algorithm = normalize_hash_algorithm(
            prompt("Verification hash (off, blake3, sha1, sha256)", default_hash_algorithm())
        )
    if cli_flag_provided("--skip-hidden", "--no-skip-hidden", "--include-hidden"):
        skip_hidden = args.skip_hidden and not args.include_hidden
    else:
        skip_hidden = prompt_yes_no("Skip hidden files?", default=True)
    if cli_flag_provided("--skip-system", "--no-skip-system", "--include-system"):
        skip_system = args.skip_system and not args.include_system
    else:
        skip_system = prompt_yes_no("Skip common system files?", default=True)
    exclude_raw = args.exclude_exts or prompt("Exclude extensions (comma-separated)", "")
    excluded_exts = normalize_exts(exclude_raw)
    if cli_flag_provided("--xlsx", "--no-xlsx"):
        create_xlsx = args.create_xlsx
    else:
        create_xlsx = prompt_yes_no("Create XLSX after the CSV scan?", default=True)
    preserve_zeros = False
    delete_csv_after_xlsx = False
    if create_xlsx:
        if cli_flag_provided("--preserve-zeros", "--no-preserve-zeros"):
            preserve_zeros = args.preserve_zeros
        else:
            preserve_zeros = prompt_yes_no("Preserve leading zeros in XLSX?", default=True)
        if cli_flag_provided("--delete-csv-after-xlsx", "--no-delete-csv-after-xlsx"):
            delete_csv_after_xlsx = args.delete_csv_after_xlsx
        else:
            delete_csv_after_xlsx = prompt_yes_no("Delete CSV after creating XLSX?", default=True)

    output_path = output_dir / output_name
    first_csv_output_path = csv_output_path_for_part(output_path, 1)
    if first_csv_output_path.exists() and not args.overwrite:
        overwrite = prompt_yes_no(f"{first_csv_output_path} already exists. Overwrite?", default=False)
        if not overwrite:
            print("Canceled.")
            return 1

    if args.max_rows_per_csv <= 0:
        print("--max-rows-per-csv must be greater than 0", file=sys.stderr)
        return 1
    agency_fields = {
        "rg": args.agency_rg,
        "sg": args.agency_sg,
        "series": args.agency_series,
        "rc_series": args.agency_rc_series,
        "dept_organization": args.agency_dept_organization,
        "division": args.agency_division,
        "section": args.agency_section,
        "unit": args.agency_unit,
        "rc_series_name": args.agency_rc_series_name,
        "begin_date": args.agency_begin_date,
        "end_date": args.agency_end_date,
        "description": args.agency_description,
        "location": args.agency_location,
        "material_type": args.agency_material_type,
        "comments": args.agency_comments,
        "confidential": args.agency_confidential,
        "disposition_date": args.agency_disposition_date,
        "box_num": args.agency_box_num,
        "td_num": args.agency_td_num,
        "location_id": args.agency_location_id,
        "record_level": args.agency_record_level,
    }
    if args.agency_template and not agency_fields["rc_series"].strip():
        print("--agency-rc-series is required when --agency-template is used", file=sys.stderr)
        return 1
    if args.agency_template:
        try:
            agency_fields = normalize_agency_template_fields(agency_fields)
        except ValueError as exc:
            print(str(exc), file=sys.stderr)
            return 1

    if hash_algorithm == "blake3" and not is_blake3_available():
        print(
            "BLAKE3 was selected, but the Python 'blake3' package is not installed.\n"
            "Install it with: pip install -r requirements.txt",
            file=sys.stderr,
        )
        return 1

    print("\nCollecting files...")
    result = run_scan(
        source_dir,
        output_path,
        hash_algorithm=hash_algorithm,
        include_hidden=not skip_hidden,
        include_system=not skip_system,
        excluded_exts=excluded_exts,
        create_xlsx=create_xlsx,
        preserve_zeros=preserve_zeros,
        delete_csv=delete_csv_after_xlsx,
        max_rows_per_csv=args.max_rows_per_csv,
        agency_template=args.agency_template,
        agency_fields=agency_fields,
    )
    print(build_scan_summary(result))
    return 0


def run_cli_email_copy(args: argparse.Namespace) -> int:
    source_raw = args.source or prompt("Source folder", str(Path.cwd()))
    dest_raw = args.dest or prompt("Destination folder")

    if not source_raw:
        print("Source folder is required.", file=sys.stderr)
        return 1
    if not dest_raw:
        print("Destination folder is required.", file=sys.stderr)
        return 1

    result = copy_email_files(
        Path(source_raw).expanduser().resolve(),
        Path(dest_raw).expanduser().resolve(),
    )
    print("\nDone")
    print(f"Source: {result.source_dir}")
    print(f"Destination: {result.dest_dir}")
    print(f"Copied: {result.copied}")
    print(f"Manifest: {result.manifest_path}")
    print("Extensions included:")
    print("  " + ", ".join(sorted(EMAIL_EXTENSIONS)))
    print("")
    print("Mode: preserve relative folders from the chosen source root")
    return 0


class EmailCopyPage:
    def __init__(self, parent: "ContentListApp", host) -> None:
        self.parent = parent
        self.page = parent.build_scrollable_root(host)
        self.page.pack_forget()

        self.message_queue: queue.Queue[tuple[str, object]] = queue.Queue()
        self.running = False
        self.latest_manifest = ""

        self.source_var = tk.StringVar(value=parent.source_var.get() or os.getcwd())
        self.dest_var = tk.StringVar(value=parent.output_dir_var.get() or os.getcwd())
        self.status_var = tk.StringVar(value="Choose a folder to search, then choose where the copied email files should go.")
        self.detail_var = tk.StringVar(value="The app will first look for supported email file types, then copy the matches and save a report.")
        self.phase_var = tk.StringVar(value="Idle")
        self.percent_var = tk.StringVar(value="0%")
        self.scanned_var = tk.StringVar(value="0")
        self.matched_var = tk.StringVar(value="0")
        self.copied_var = tk.StringVar(value="0")
        self.start_button: ctk.CTkButton | None = None
        self.reset_button: ctk.CTkButton | None = None
        self.source_entry: ctk.CTkEntry | None = None
        self.dest_entry: ctk.CTkEntry | None = None
        self.progress: ctk.CTkProgressBar | None = None
        self.summary_box: ctk.CTkTextbox | None = None
        self.manifest_button: ctk.CTkButton | None = None

        self.build_ui()
        self.page.after(100, self.pump_queue)

    def build_ui(self) -> None:
        outer = self.page
        outer.grid_columnconfigure(0, weight=1)
        outer.grid_columnconfigure(1, weight=1)

        hero = ctk.CTkFrame(outer, fg_color=themed_color("hero_card_bg"), corner_radius=22)
        hero.grid(row=0, column=0, columnspan=2, sticky="ew", padx=28, pady=(28, 0))
        hero.grid_columnconfigure(0, weight=1)
        ctk.CTkLabel(
            hero,
            text="Copy Email Files",
            font=ctk.CTkFont(size=30, weight="bold"),
            text_color=themed_color("title_fg"),
        ).grid(row=0, column=0, sticky="w", padx=28, pady=(24, 0))
        ctk.CTkLabel(
            hero,
            text="Choose a folder to search, choose where the copied files should go, and the app will save a report of everything that was copied.",
            text_color=themed_color("hint_fg"),
            font=ctk.CTkFont(size=14),
            wraplength=760,
            justify="left",
        ).grid(row=1, column=0, sticky="w", padx=28, pady=(8, 24))

        hero_status = ctk.CTkFrame(hero, fg_color=themed_color("card_bg"), corner_radius=18)
        hero_status.grid(row=0, column=1, rowspan=2, sticky="ne", padx=24, pady=24)
        ctk.CTkLabel(hero_status, textvariable=self.phase_var, font=ctk.CTkFont(size=12, weight="bold"), text_color=themed_color("hint_fg")).pack(anchor="e", padx=18, pady=(14, 0))
        ctk.CTkLabel(hero_status, textvariable=self.percent_var, font=ctk.CTkFont(size=28, weight="bold"), text_color=themed_color("body_fg")).pack(anchor="e", padx=18, pady=(4, 14))

        input_card = ctk.CTkFrame(outer, fg_color=themed_color("card_bg"), corner_radius=22)
        input_card.grid(row=1, column=0, sticky="nsew", padx=(28, 10), pady=(18, 0))
        input_card.grid_columnconfigure(0, weight=1)

        ctk.CTkLabel(input_card, text="Folders", font=ctk.CTkFont(size=20, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", padx=24, pady=(22, 4))
        ctk.CTkLabel(
            input_card,
            text="The copied files keep the same folder structure they had in the folder you search.",
            font=ctk.CTkFont(size=13),
            text_color=themed_color("hint_fg"),
            wraplength=660,
            justify="left",
        ).grid(row=1, column=0, sticky="w", padx=24, pady=(0, 16))

        source_card = self.parent.make_field_card(input_card, "Folder to search", self.source_var, self.choose_source)
        source_card.grid(row=2, column=0, padx=24, pady=(0, 14), sticky="ew")
        self.source_entry = source_card.entry
        dest_card = self.parent.make_field_card(input_card, "Copy files into", self.dest_var, self.choose_dest)
        dest_card.grid(row=3, column=0, padx=24, pady=(0, 18), sticky="ew")
        self.dest_entry = dest_card.entry

        actions = ctk.CTkFrame(input_card, fg_color="transparent")
        actions.grid(row=4, column=0, sticky="ew", padx=24, pady=(0, 22))
        self.start_button = self.parent.make_primary_button(actions, "Copy Email Files", self.start_copy)
        self.start_button.pack(side="left")
        self.parent.make_secondary_button(actions, "Use Main Output Folder", self.use_main_output).pack(side="left", padx=(10, 0))
        self.reset_button = self.parent.make_secondary_button(actions, "Reset", self.reset_fields)
        self.reset_button.pack(side="left", padx=(10, 0))
        self.parent.make_secondary_button(actions, "Back to Content List", lambda: self.parent.show_page("content")).pack(side="left", padx=(10, 0))

        side_card = ctk.CTkFrame(outer, fg_color=themed_color("card_bg"), corner_radius=22)
        side_card.grid(row=1, column=1, sticky="nsew", padx=(10, 28), pady=(18, 0))
        side_card.grid_columnconfigure(0, weight=1)

        ctk.CTkLabel(side_card, text="Supported email file types", font=ctk.CTkFont(size=20, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", padx=24, pady=(22, 6))
        chip_box = ctk.CTkFrame(side_card, fg_color=themed_color("chip_bg"), corner_radius=18)
        chip_box.grid(row=1, column=0, sticky="ew", padx=24)
        chip_text = "\n".join(sorted(EMAIL_EXTENSIONS))
        ctk.CTkLabel(
            chip_box,
            text=chip_text,
            font=ctk.CTkFont(size=13, family="Menlo"),
            text_color=themed_color("body_fg"),
            justify="left",
        ).pack(anchor="w", padx=18, pady=16)
        ctk.CTkLabel(
            side_card,
            text="The app checks folders for these file types first, then copies the matches and writes a report.",
            text_color=themed_color("hint_fg"),
            font=ctk.CTkFont(size=12),
            wraplength=260,
            justify="left",
        ).grid(row=2, column=0, sticky="w", padx=24, pady=(14, 18))

        stats = ctk.CTkFrame(side_card, fg_color="transparent")
        stats.grid(row=3, column=0, sticky="ew", padx=24, pady=(0, 22))
        stats.grid_columnconfigure((0, 1, 2), weight=1)
        self.parent.make_metric_card(stats, "Files Checked", self.scanned_var).grid(row=0, column=0, sticky="ew", padx=(0, 8))
        self.parent.make_metric_card(stats, "Matches Found", self.matched_var, accent=True).grid(row=0, column=1, sticky="ew", padx=4)
        self.parent.make_metric_card(stats, "Files Copied", self.copied_var, accent=True).grid(row=0, column=2, sticky="ew", padx=(8, 0))

        progress_card = ctk.CTkFrame(outer, fg_color=themed_color("card_bg"), corner_radius=22)
        progress_card.grid(row=2, column=0, columnspan=2, sticky="ew", padx=28, pady=(18, 0))
        progress_card.grid_columnconfigure(0, weight=1)
        ctk.CTkLabel(progress_card, text="Progress", font=ctk.CTkFont(size=18, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", padx=24, pady=(20, 8))
        self.progress = ctk.CTkProgressBar(progress_card, progress_color=themed_color("progress_fill"), mode="determinate")
        self.progress.grid(row=1, column=0, sticky="ew", padx=24)
        self.progress.set(0)
        ctk.CTkLabel(progress_card, textvariable=self.status_var, text_color=themed_color("body_fg"), wraplength=980, justify="left").grid(row=2, column=0, sticky="w", padx=24, pady=(12, 0))
        ctk.CTkLabel(progress_card, textvariable=self.detail_var, text_color=themed_color("hint_fg"), wraplength=980, justify="left").grid(row=3, column=0, sticky="w", padx=24, pady=(6, 18))

        summary_card = ctk.CTkFrame(outer, fg_color=themed_color("card_bg"), corner_radius=22)
        summary_card.grid(row=3, column=0, columnspan=2, sticky="nsew", padx=28, pady=(18, 28))
        summary_card.grid_columnconfigure(0, weight=1)
        ctk.CTkLabel(summary_card, text="Copy Summary", font=ctk.CTkFont(size=18, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", padx=24, pady=(20, 8))
        self.summary_box = ctk.CTkTextbox(
            summary_card,
            height=180,
            wrap="word",
            fg_color=themed_color("card_alt_bg"),
            text_color=themed_color("body_fg"),
            border_width=0,
            font=("Menlo", 11),
        )
        self.summary_box.grid(row=1, column=0, sticky="nsew", padx=24, pady=(0, 18))
        self.summary_box.insert("1.0", "Your copy summary will appear here after the job is finished.")
        self.summary_box.configure(state="disabled")

        footer_actions = ctk.CTkFrame(summary_card, fg_color="transparent")
        footer_actions.grid(row=2, column=0, sticky="e", padx=24, pady=(0, 20))
        self.manifest_button = self.parent.make_secondary_button(footer_actions, "Open Manifest", self.open_manifest)
        self.manifest_button.pack(side="left")
        self.manifest_button.configure(state="disabled")
        self.parent.make_secondary_button(footer_actions, "Open Destination", self.open_destination).pack(side="left", padx=(10, 0))

    def focus_source_entry(self) -> None:
        if self.source_entry is not None and self.source_entry.winfo_exists():
            self.source_entry.focus_force()
            self.source_entry.icursor("end")

    def choose_source(self) -> None:
        chosen = choose_directory(self.parent.root, "Choose Source Folder", self.source_var.get(), True, self.parent.colors)
        if chosen:
            self.source_var.set(chosen)
            self.status_var.set("Folder selected. Now choose where the copied files should go.")
            if self.dest_entry is not None:
                self.dest_entry.focus_force()
                self.dest_entry.icursor("end")

    def choose_dest(self) -> None:
        chosen = choose_directory(self.parent.root, "Choose Destination Folder", self.dest_var.get(), False, self.parent.colors)
        if chosen:
            self.dest_var.set(chosen)
            self.status_var.set("Destination selected. Click Start Copy when you're ready.")

    def reset_fields(self) -> None:
        self.source_var.set(self.parent.source_var.get() or os.getcwd())
        self.dest_var.set(self.parent.output_dir_var.get() or os.getcwd())
        self.status_var.set("Choose a folder to search, then choose where the copied email files should go.")
        self.detail_var.set("The app will first look for supported email file types, then copy the matches and save a report.")
        self.phase_var.set("Idle")
        self.percent_var.set("0%")
        self.scanned_var.set("0")
        self.matched_var.set("0")
        self.copied_var.set("0")
        self.latest_manifest = ""
        if self.progress is not None:
            self.set_progress_mode("determinate")
            self.progress.set(0)
        if self.manifest_button is not None:
            self.manifest_button.configure(state="disabled")
        self.set_summary("Your copy summary will appear here after the job is finished.")
        self.focus_source_entry()

    def use_main_output(self) -> None:
        self.dest_var.set(self.parent.output_dir_var.get() or os.getcwd())
        self.status_var.set("Using the main window results folder as the destination.")

    def start_copy(self) -> None:
        if self.running:
            return

        source = Path(self.source_var.get()).expanduser().resolve()
        dest = Path(self.dest_var.get()).expanduser().resolve()
        if not source.is_dir():
            messagebox.showerror("Invalid source folder", f"Source folder does not exist:\n{source}")
            return

        self.running = True
        if self.progress is not None:
            self.set_progress_mode("indeterminate")
            self.progress.set(0)
            self.progress.start()
        self.phase_var.set("Scanning")
        self.percent_var.set("Scanning")
        self.status_var.set("Looking for supported email files...")
        self.detail_var.set("Checking folders for supported email file types before the copy begins.")
        self.scanned_var.set("0")
        self.matched_var.set("0")
        self.copied_var.set("0")
        self.set_summary("Preparing the copy job...")
        if self.start_button is not None:
            self.start_button.configure(state="disabled")
        if self.reset_button is not None:
            self.reset_button.configure(state="disabled")
        if self.manifest_button is not None:
            self.manifest_button.configure(state="disabled")
        thread = threading.Thread(target=self.run_copy_thread, args=(source, dest), daemon=True)
        thread.start()

    def run_copy_thread(self, source: Path, dest: Path) -> None:
        try:
            def on_progress(progress: EmailCopyProgress) -> None:
                self.message_queue.put(("progress", progress))

            result = copy_email_files(source, dest, progress_callback=on_progress)
            self.message_queue.put(("done", result))
        except Exception as exc:  # pragma: no cover
            self.message_queue.put(("error", str(exc)))

    def pump_queue(self) -> None:
        try:
            while True:
                kind, payload = self.message_queue.get_nowait()
                if kind == "done":
                    self.running = False
                    if self.start_button is not None:
                        self.start_button.configure(state="normal")
                    if self.reset_button is not None:
                        self.reset_button.configure(state="normal")
                    result: EmailCopyResult = payload
                    self.latest_manifest = str(result.manifest_path)
                    self.phase_var.set("Complete")
                    self.percent_var.set("100%")
                    self.parent.status_var.set(
                        f"Done. Copied {result.copied} email files to {result.dest_dir}."
                    )
                    if self.progress is not None:
                        self.progress.stop()
                        self.set_progress_mode("determinate")
                        self.progress.set(1)
                    if self.manifest_button is not None:
                        self.manifest_button.configure(state="normal")
                    self.set_summary(
                        "\n".join(
                            [
                                "Copy Email Files Complete",
                                f"Searched folder: {result.source_dir}",
                                f"Copied files to: {result.dest_dir}",
                                f"Report saved to: {result.manifest_path}",
                                f"Email files copied: {result.copied}",
                                f"Finished in: {result.elapsed:.2f}s",
                                "",
                                "Supported email file types:",
                                ", ".join(sorted(EMAIL_EXTENSIONS)),
                            ]
                        )
                    )
                    self.parent.append_summary(
                        "\n".join(
                            [
                                "Copy Email Files Complete",
                                f"Searched folder: {result.source_dir}",
                                f"Copied files to: {result.dest_dir}",
                                f"Report saved to: {result.manifest_path}",
                                f"Email files copied: {result.copied}",
                                f"Finished in: {result.elapsed:.2f}s",
                            ]
                        )
                    )
                    self.detail_var.set("The report was saved and the original folder structure was preserved.")
                    messagebox.showinfo(
                        "Done",
                        f"Copied {result.copied} files.\n\nDestination: {result.dest_dir}\nManifest: {result.manifest_path}",
                    )
                if kind == "progress":
                    progress: EmailCopyProgress = payload
                    self.scanned_var.set(str(progress.scanned))
                    self.matched_var.set(str(progress.matched))
                    if progress.phase == "scanning":
                        self.phase_var.set("Scanning")
                        self.percent_var.set("Scanning")
                        self.status_var.set(
                            f"Checking files... Looked at: {progress.scanned}  Matches found: {progress.matched}"
                        )
                        if progress.current_name:
                            self.detail_var.set(f"Checking: {progress.current_name}")
                        else:
                            self.detail_var.set("Checking folders for supported email file types.")
                    else:
                        self.phase_var.set("Copying")
                        total = max(1, progress.total)
                        if self.progress is not None:
                            self.progress.stop()
                            self.set_progress_mode("determinate")
                            self.progress.set(progress.copied / total)
                        self.percent_var.set(f"{int((progress.copied / total) * 100)}%")
                        self.copied_var.set(str(progress.copied))
                        if progress.total == 0:
                            self.status_var.set(
                                f"Finished checking {progress.scanned} files. No supported email files were found."
                            )
                            self.detail_var.set("Nothing matched the supported email file types in this folder.")
                        elif progress.current_relative:
                            self.status_var.set(
                                f"Copying files... {progress.copied} of {progress.total}: {progress.current_relative}"
                            )
                            self.detail_var.set(
                                f"Found {progress.total} supported email files after checking {progress.scanned} files."
                            )
                        else:
                            self.status_var.set(
                                f"Found {progress.total} supported email files after checking {progress.scanned} files."
                            )
                            self.detail_var.set("Starting the copy now.")
                if kind == "error":
                    self.running = False
                    if self.start_button is not None:
                        self.start_button.configure(state="normal")
                    if self.reset_button is not None:
                        self.reset_button.configure(state="normal")
                    if self.progress is not None:
                        self.progress.stop()
                        self.set_progress_mode("determinate")
                    self.status_var.set("Something went wrong while copying the email files.")
                    self.phase_var.set("Error")
                    self.percent_var.set("0%")
                    messagebox.showerror("Copy failed", str(payload))
        except queue.Empty:
            pass
        if self.page.winfo_exists():
            self.page.after(100, self.pump_queue)

    def set_progress_mode(self, mode: str) -> None:
        if self.progress is None:
            return
        self.progress.configure(mode=mode)

    def set_summary(self, text: str) -> None:
        if self.summary_box is None:
            return
        self.summary_box.configure(state="normal")
        self.summary_box.delete("1.0", "end")
        self.summary_box.insert("end", text)
        self.summary_box.configure(state="disabled")

    def open_manifest(self) -> None:
        if self.latest_manifest:
            open_in_file_manager(Path(self.latest_manifest))

    def open_destination(self) -> None:
        open_in_file_manager(Path(self.dest_var.get() or os.getcwd()))


class ContentListApp:
    def __init__(self) -> None:
        current_mode = load_theme_mode()
        ctk.set_default_color_theme("blue")
        ctk.set_appearance_mode(theme_mode_label(current_mode))
        self.root = ctk.CTk()
        self.root.title("Content List Toolkit")
        self.root.geometry("1360x860")
        self.root.minsize(1160, 760)
        self.theme_mode_var = tk.StringVar(value=theme_mode_label(current_mode))
        self.colors = palette_for_mode(effective_theme_mode(current_mode))
        self.root.configure(fg_color=themed_color("app_bg"))

        self.message_queue: queue.Queue[tuple[str, object]] = queue.Queue()
        self.running = False
        self.active_page = "content"

        cwd = Path(os.getcwd())
        self.source_var = tk.StringVar(value=str(cwd))
        self.output_dir_var = tk.StringVar(value=str(default_scan_output_dir(cwd)))
        self.output_name_var = tk.StringVar(value=default_output_name(cwd))
        self.exclude_var = tk.StringVar(value="")
        self.hash_algorithm_var = tk.StringVar(value=hash_algorithm_label(default_hash_algorithm()))
        self.clone_verify_var = tk.BooleanVar(value=False)
        self.clone_note_var = tk.StringVar(value="Turn this on to scan the 1st Drive first, then choose the 2nd Drive after the 1st Drive finishes.")
        self.hidden_var = tk.BooleanVar(value=True)
        self.system_var = tk.BooleanVar(value=True)
        self.folders_only_var = tk.BooleanVar(value=False)
        self.folder_depth_var = tk.StringVar(value="0")
        self.xlsx_var = tk.BooleanVar(value=True)
        self.preserve_zeros_var = tk.BooleanVar(value=True)
        self.delete_csv_var = tk.BooleanVar(value=True)
        self.agency_template_var = tk.BooleanVar(value=False)
        self.agency_field_vars = {
            "rg": tk.StringVar(value=""),
            "sg": tk.StringVar(value=""),
            "series": tk.StringVar(value=""),
            "rc_series": tk.StringVar(value=""),
            "dept_organization": tk.StringVar(value=""),
            "division": tk.StringVar(value=""),
            "section": tk.StringVar(value=""),
            "unit": tk.StringVar(value=""),
            "rc_series_name": tk.StringVar(value=""),
            "begin_date": tk.StringVar(value=""),
            "end_date": tk.StringVar(value=""),
            "description": tk.StringVar(value=""),
            "location": tk.StringVar(value=""),
            "material_type": tk.StringVar(value="Born Digital"),
            "comments": tk.StringVar(value=""),
            "confidential": tk.StringVar(value=""),
            "disposition_date": tk.StringVar(value=""),
            "box_num": tk.StringVar(value=""),
            "td_num": tk.StringVar(value=""),
            "location_id": tk.StringVar(value=""),
            "record_level": tk.StringVar(value="Item"),
        }
        self.status_var = tk.StringVar(value="Choose a folder to scan, then click Generate.")
        self.scan_files_var = tk.StringVar(value="0")
        self.scan_skipped_var = tk.StringVar(value="0")
        self.scan_saved_var = tk.StringVar(value="Waiting")
        self.generate_button: ctk.CTkButton | None = None
        self.stop_button: ctk.CTkButton | None = None
        self.about_button: ctk.CTkButton | None = None
        self.open_folder_button: ctk.CTkButton | None = None
        self.reset_button: ctk.CTkButton | None = None
        self.source_entry: ctk.CTkEntry | None = None
        self.output_entry: ctk.CTkEntry | None = None
        self.file_entry: ctk.CTkEntry | None = None
        self.exclude_entry: ctk.CTkEntry | None = None
        self.summary: ctk.CTkTextbox | None = None
        self.progress: ctk.CTkProgressBar | None = None
        self.hash_select_tile: ctk.CTkFrame | None = None
        self.scan_cancel_event: threading.Event | None = None
        self.folders_only_toggle: ctk.CTkCheckBox | None = None
        self.folder_depth_frame: ctk.CTkFrame | None = None
        self.folder_depth_entry: ctk.CTkEntry | None = None
        self.preserve_zeros_toggle: ctk.CTkCheckBox | None = None
        self.delete_csv_toggle: ctk.CTkCheckBox | None = None
        self.delete_csv_tile: ctk.CTkFrame | None = None
        self.agency_template_toggle: ctk.CTkCheckBox | None = None
        self.pending_clone_result: CloneVerificationResult | None = None
        self.page_frames: dict[str, ctk.CTkFrame] = {}
        self.nav_buttons: dict[str, ctk.CTkButton] = {}
        self.email_page = None

        self.configure_style()
        self.build_ui()
        self.bind_app_scrolling()
        self.root.after(50, self.focus_source_entry)
        self.root.after(100, self.pump_queue)

    def configure_style(self) -> None:
        colors = self.colors
        style = ttk.Style()
        if "clam" in style.theme_names():
            style.theme_use("clam")
        style.configure("App.TFrame", background=colors["app_bg"])
        style.configure("Card.TFrame", background=colors["card_bg"], relief="flat")
        style.configure("Panel.TFrame", background=colors["card_alt_bg"], relief="flat")
        style.configure("Title.TLabel", background=colors["app_bg"], foreground=colors["title_fg"], font=("Segoe UI", 26, "bold"))
        style.configure("Body.TLabel", background=colors["card_bg"], foreground=colors["body_fg"], font=("Segoe UI", 11))
        style.configure("AppBody.TLabel", background=colors["app_bg"], foreground=colors["body_fg"], font=("Segoe UI", 11))
        style.configure("Hint.TLabel", background=colors["app_bg"], foreground=colors["hint_fg"], font=("Segoe UI", 10))
        style.configure("CardHint.TLabel", background=colors["card_bg"], foreground=colors["hint_fg"], font=("Segoe UI", 10))
        style.configure(
            "Primary.TButton",
            font=("Segoe UI", 11, "bold"),
            background=colors["primary_bg"],
            foreground=colors["primary_fg"],
            bordercolor=colors["primary_bg"],
            focuscolor=colors["primary_bg"],
            lightcolor=colors["primary_bg"],
            darkcolor=colors["primary_bg"],
        )
        style.map(
            "Primary.TButton",
            background=[("active", colors["primary_hover"]), ("disabled", colors["border"])],
            foreground=[("disabled", colors["hero_muted"])],
        )
        style.configure(
            "Secondary.TButton",
            font=("Segoe UI", 10),
            background=colors["secondary_bg"],
            foreground=colors["secondary_fg"],
            bordercolor=colors["border"],
            focuscolor=colors["secondary_bg"],
            lightcolor=colors["secondary_bg"],
            darkcolor=colors["secondary_bg"],
        )
        style.map(
            "Secondary.TButton",
            background=[("active", colors["secondary_hover"]), ("disabled", colors["border"])],
            foreground=[("disabled", colors["hint_fg"])],
        )
        style.configure(
            "App.TEntry",
            fieldbackground=colors["entry_bg"],
            foreground=colors["entry_fg"],
            insertcolor=colors["entry_fg"],
            bordercolor=colors["border"],
            lightcolor=colors["border"],
            darkcolor=colors["border"],
        )
        style.map(
            "App.TEntry",
            fieldbackground=[("disabled", colors["card_alt_bg"]), ("!disabled", colors["entry_bg"])],
            foreground=[("disabled", colors["hint_fg"]), ("!disabled", colors["entry_fg"])],
        )
        style.configure(
            "Modern.Horizontal.TProgressbar",
            troughcolor=colors["progress_trough"],
            background=colors["progress_fill"],
            bordercolor=colors["progress_trough"],
        )

    def build_ui(self) -> None:
        mismatches = check_deps()
        if mismatches:
            banner = ctk.CTkLabel(
                self.root,
                text=format_banner_text(mismatches),
                fg_color="#b97a00",
                text_color="#ffffff",
                anchor="w",
                justify="left",
                font=("Segoe UI", 11, "bold"),
                corner_radius=0,
            )
            banner.pack(fill="x", side="top", ipadx=12, ipady=6)

        shell = ctk.CTkFrame(self.root, fg_color=themed_color("app_bg"), corner_radius=0)
        shell.pack(fill="both", expand=True)

        sidebar = ctk.CTkFrame(shell, width=304, fg_color=themed_color("sidebar_bg"), corner_radius=18)
        sidebar.pack(side="left", fill="y", padx=(18, 10), pady=18)
        sidebar.pack_propagate(False)

        content_shell = ctk.CTkFrame(shell, fg_color=themed_color("app_bg"), corner_radius=0)
        content_shell.pack(side="left", fill="both", expand=True, padx=(10, 18), pady=18)

        self.build_sidebar(sidebar)
        self.page_frames["content"] = self.build_content_page(content_shell)
        self.page_frames["email"] = self.build_email_page(content_shell)
        self.page_frames["about"] = self.build_about_page(content_shell)
        self.show_page("content")
        self.sync_xlsx_state()
        self.sync_action_buttons()

    def build_scrollable_root(self, parent) -> ctk.CTkScrollableFrame:
        shell = ctk.CTkScrollableFrame(parent, fg_color=themed_color("app_bg"), corner_radius=0)
        shell.pack(fill="both", expand=True, padx=0, pady=0)
        return shell

    def bind_app_scrolling(self) -> None:
        self.root.bind_all("<MouseWheel>", self.on_mousewheel, add="+")
        self.root.bind_all("<Shift-MouseWheel>", self.on_shift_mousewheel, add="+")
        self.root.bind_all("<Button-4>", self.on_linux_scroll_up, add="+")
        self.root.bind_all("<Button-5>", self.on_linux_scroll_down, add="+")
        self.root.bind_all("<Up>", self.on_arrow_up, add="+")
        self.root.bind_all("<Down>", self.on_arrow_down, add="+")
        self.root.bind_all("<Prior>", self.on_page_up, add="+")
        self.root.bind_all("<Next>", self.on_page_down, add="+")

    def active_scrollable(self):
        frame = self.page_frames.get(self.active_page)
        if frame is None:
            return None
        return frame

    def focused_widget_class(self) -> str:
        try:
            widget = self.root.focus_get()
            if widget is None:
                return ""
            return str(widget.winfo_class()).lower()
        except Exception:
            return ""

    def should_preserve_arrow_key(self) -> bool:
        widget_class = self.focused_widget_class()
        return any(name in widget_class for name in ("entry", "text", "listbox", "spinbox"))

    def scroll_active(self, units: int, axis: str = "y", what: str = "units") -> str:
        scrollable = self.active_scrollable()
        if scrollable is None:
            return "break"
        try:
            canvas = scrollable._parent_canvas
            if axis == "x":
                canvas.xview_scroll(units, what)
            else:
                canvas.yview_scroll(units, what)
        except Exception:
            return "break"
        return "break"

    def on_mousewheel(self, event) -> str:
        if sys.platform == "darwin":
            delta = -1 * int(event.delta)
            if delta == 0:
                delta = -1 if event.delta > 0 else 1
            return self.scroll_active(delta)
        step = -1 * int(event.delta / 120) if event.delta else 0
        if step == 0:
            step = -1 if event.delta > 0 else 1
        return self.scroll_active(step)

    def on_shift_mousewheel(self, event) -> str:
        if sys.platform == "darwin":
            delta = -1 * int(event.delta)
            if delta == 0:
                delta = -1 if event.delta > 0 else 1
            return self.scroll_active(delta, axis="x")
        step = -1 * int(event.delta / 120) if event.delta else 0
        if step == 0:
            step = -1 if event.delta > 0 else 1
        return self.scroll_active(step, axis="x")

    def on_linux_scroll_up(self, _event) -> str:
        return self.scroll_active(-3)

    def on_linux_scroll_down(self, _event) -> str:
        return self.scroll_active(3)

    def on_arrow_up(self, _event) -> str | None:
        if self.should_preserve_arrow_key():
            return None
        return self.scroll_active(-2)

    def on_arrow_down(self, _event) -> str | None:
        if self.should_preserve_arrow_key():
            return None
        return self.scroll_active(2)

    def on_page_up(self, _event) -> str | None:
        if self.should_preserve_arrow_key():
            return None
        return self.scroll_active(-1, what="pages")

    def on_page_down(self, _event) -> str | None:
        if self.should_preserve_arrow_key():
            return None
        return self.scroll_active(1, what="pages")

    def build_sidebar(self, parent) -> None:
        brand = ctk.CTkFrame(parent, fg_color="transparent")
        brand.pack(fill="x", padx=18, pady=(24, 20))
        ctk.CTkLabel(
            brand,
            text="Content List Toolkit",
            font=ctk.CTkFont(size=20, weight="bold"),
            text_color=themed_color("title_fg"),
            wraplength=230,
            justify="left",
        ).pack(anchor="w")
        ctk.CTkLabel(
            brand,
            text="Create file lists, copy email files, and keep a simple record of what was saved.",
            font=ctk.CTkFont(size=13),
            text_color=themed_color("hint_fg"),
            wraplength=220,
            justify="left",
        ).pack(anchor="w", pady=(10, 0))

        nav = ctk.CTkFrame(parent, fg_color="transparent")
        nav.pack(fill="x", padx=16, pady=(4, 0))
        self.nav_buttons["content"] = self.make_nav_button(nav, "Content List", lambda: self.show_page("content"))
        self.nav_buttons["content"].pack(fill="x", pady=4)
        self.nav_buttons["email"] = self.make_nav_button(nav, "Copy Email Files", lambda: self.show_page("email"))
        self.nav_buttons["email"].pack(fill="x", pady=4)
        self.nav_buttons["about"] = self.make_nav_button(nav, "About", lambda: self.show_page("about"))
        self.nav_buttons["about"].pack(fill="x", pady=4)

        footer = ctk.CTkFrame(parent, fg_color=themed_color("card_bg"), corner_radius=18)
        footer.pack(side="bottom", fill="x", padx=16, pady=20)
        ctk.CTkLabel(
            footer,
            text="Appearance",
            font=ctk.CTkFont(size=12, weight="bold"),
            text_color=themed_color("hint_fg"),
        ).pack(anchor="w", padx=16, pady=(16, 6))
        appearance_menu = ctk.CTkOptionMenu(
            footer,
            values=[THEME_MODE_LABELS[THEME_MODE_SYSTEM], THEME_MODE_LABELS[THEME_MODE_LIGHT], THEME_MODE_LABELS[THEME_MODE_DARK]],
            variable=self.theme_mode_var,
            command=lambda _value: self.toggle_theme(),
            fg_color=themed_color("secondary_bg"),
            button_color=themed_color("primary_bg"),
            button_hover_color=themed_color("primary_hover"),
            text_color=themed_color("body_fg"),
            dropdown_fg_color=themed_color("card_bg"),
            dropdown_hover_color=themed_color("secondary_hover"),
            dropdown_text_color=themed_color("body_fg"),
            corner_radius=10,
        )
        appearance_menu.pack(fill="x", padx=16, pady=(0, 12))
        ctk.CTkLabel(
            footer,
            text="Choose System, Light, or Dark. System follows the desktop appearance when supported.",
            font=ctk.CTkFont(size=12),
            text_color=themed_color("hint_fg"),
            wraplength=196,
            justify="left",
        ).pack(anchor="w", padx=16, pady=(0, 14))

    def build_content_page(self, parent) -> ctk.CTkScrollableFrame:
        page = self.build_scrollable_root(parent)
        page.pack_forget()
        page.grid_columnconfigure(0, weight=1)
        page.grid_columnconfigure(1, weight=1)

        hero = ctk.CTkFrame(page, fg_color=themed_color("hero_bg"), corner_radius=24)
        hero.grid(row=0, column=0, columnspan=2, sticky="ew", padx=28, pady=(28, 0))
        hero.grid_columnconfigure(0, weight=1)
        ctk.CTkLabel(
            hero,
            text="Content List Toolkit",
            font=ctk.CTkFont(size=36, weight="bold"),
            text_color=themed_color("hero_fg"),
        ).grid(row=0, column=0, sticky="w", padx=28, pady=(24, 0))
        ctk.CTkLabel(
            hero,
            text=(
                "Create a simple file list from a folder and save an Excel copy if you want one."
            ),
            font=ctk.CTkFont(size=14),
            text_color=themed_color("hero_muted"),
            wraplength=780,
            justify="left",
        ).grid(row=1, column=0, sticky="w", padx=28, pady=(10, 24))

        source_card = self.make_field_card(page, "Folder to scan", self.source_var, self.choose_source, hint="Choose the folder you want the app to scan.")
        source_card.grid(row=1, column=0, sticky="nsew", padx=(28, 10), pady=(22, 0))
        self.source_entry = source_card.entry
        output_card = self.make_field_card(page, "Save results to", self.output_dir_var, self.choose_output, hint="Choose where the CSV file should be saved.")
        output_card.grid(row=1, column=1, sticky="nsew", padx=(10, 28), pady=(22, 0))
        self.output_entry = output_card.entry

        naming_card = ctk.CTkFrame(page, fg_color=themed_color("card_bg"), corner_radius=22)
        naming_card.grid(row=2, column=0, columnspan=2, sticky="ew", padx=28, pady=(18, 0))
        naming_card.grid_columnconfigure((0, 1), weight=1)
        ctk.CTkLabel(naming_card, text="Output details", font=ctk.CTkFont(size=20, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, columnspan=2, sticky="w", padx=24, pady=(20, 12))
        ctk.CTkLabel(
            naming_card,
            text=f"Large scans split every {DEFAULT_MAX_ROWS_PER_CSV:,} rows using [name]-001.csv, [name]-002.csv, and so on.",
            font=ctk.CTkFont(size=12),
            text_color=themed_color("hint_fg"),
            wraplength=900,
            justify="left",
        ).grid(row=1, column=0, columnspan=2, sticky="w", padx=24, pady=(0, 12))
        file_card = self.make_text_field_card(naming_card, "Name for the saved list", self.output_name_var, hint="The file name should end in .csv. The first part will be saved as [name]-001.csv.")
        file_card.grid(row=2, column=0, sticky="ew", padx=(24, 10), pady=(0, 20))
        self.file_entry = file_card.entry
        exclude_card = self.make_text_field_card(naming_card, "Skip file types (optional)", self.exclude_var, hint="Example: tmp,log,bak")
        exclude_card.grid(row=2, column=1, sticky="ew", padx=(10, 24), pady=(0, 20))
        self.exclude_entry = exclude_card.entry

        clone_card = ctk.CTkFrame(page, fg_color=themed_color("hero_card_bg"), corner_radius=22)
        clone_card.grid(row=3, column=0, columnspan=2, sticky="ew", padx=28, pady=(18, 0))
        clone_card.grid_columnconfigure(0, weight=1)
        ctk.CTkLabel(
            clone_card,
            text="Clone Drive Verification",
            font=ctk.CTkFont(size=20, weight="bold"),
            text_color=themed_color("body_fg"),
        ).grid(row=0, column=0, sticky="w", padx=24, pady=(20, 8))
        ctk.CTkLabel(
            clone_card,
            text="Scan the 1st Drive, then choose the Clone/2nd Drive after the 1st Drive finishes. The app compares both content lists and only reports differences.",
            font=ctk.CTkFont(size=13),
            text_color=themed_color("hint_fg"),
            wraplength=920,
            justify="left",
        ).grid(row=1, column=0, sticky="w", padx=24, pady=(0, 10))
        self.make_option_check(clone_card, "Verify Clones", self.clone_verify_var, command=self.sync_clone_state).grid(row=2, column=0, sticky="w", padx=24, pady=(0, 8))
        ctk.CTkLabel(
            clone_card,
            textvariable=self.clone_note_var,
            font=ctk.CTkFont(size=12),
            text_color=themed_color("hint_fg"),
            wraplength=920,
            justify="left",
        ).grid(row=3, column=0, sticky="w", padx=24, pady=(0, 20))

        options_wrap = ctk.CTkFrame(page, fg_color="transparent")
        options_wrap.grid(row=4, column=0, columnspan=2, sticky="ew", padx=28, pady=(18, 0))
        options_wrap.grid_columnconfigure((0, 1), weight=1)
        ctk.CTkLabel(options_wrap, text="Options", font=ctk.CTkFont(size=20, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", pady=(0, 10))
        ctk.CTkLabel(options_wrap, text="Choose any extras you want before you generate the file list.", font=ctk.CTkFont(size=13), text_color=themed_color("hint_fg")).grid(row=1, column=0, columnspan=2, sticky="w", pady=(0, 14))

        self.hash_select_tile = self.make_select_tile(options_wrap, "Verification hash", self.hash_algorithm_var, hash_algorithm_labels())
        self.hash_select_tile.grid(row=2, column=0, sticky="ew", padx=(0, 10), pady=(0, 10))
        checks_card = ctk.CTkFrame(options_wrap, fg_color=themed_color("card_bg"), corner_radius=18)
        checks_card.grid(row=2, column=1, rowspan=3, sticky="nsew", padx=(10, 0), pady=(0, 10))
        checks_card.grid_columnconfigure(0, weight=1)
        self.folders_only_toggle = self.make_option_check(checks_card, "Folders only (no files)", self.folders_only_var, command=self.sync_folders_only_state)
        self.folders_only_toggle.pack(anchor="w", fill="x", padx=16, pady=(14, 6))
        self.folder_depth_frame = ctk.CTkFrame(checks_card, fg_color="transparent")
        self.folder_depth_frame.pack(anchor="w", fill="x", padx=32, pady=(0, 6))
        ctk.CTkLabel(self.folder_depth_frame, text="Max depth (0 = all levels):", font=ctk.CTkFont(size=13), text_color=themed_color("hint_fg")).pack(side="left", padx=(0, 8))
        self.folder_depth_entry = ctk.CTkEntry(self.folder_depth_frame, textvariable=self.folder_depth_var, width=60, font=ctk.CTkFont(size=13))
        self.folder_depth_entry.pack(side="left")
        self.folder_depth_frame.pack_forget()
        self.make_option_check(checks_card, "Skip hidden files", self.hidden_var).pack(anchor="w", fill="x", padx=16, pady=6)
        self.make_option_check(checks_card, "Skip common system files", self.system_var).pack(anchor="w", fill="x", padx=16, pady=6)
        self.make_option_check(checks_card, "Also save an Excel copy", self.xlsx_var, command=self.sync_xlsx_state).pack(anchor="w", fill="x", padx=16, pady=6)
        self.preserve_zeros_toggle = self.make_option_check(checks_card, "Keep leading zeros in Excel", self.preserve_zeros_var)
        self.preserve_zeros_toggle.pack(anchor="w", fill="x", padx=16, pady=6)
        self.delete_csv_tile = ctk.CTkFrame(checks_card, fg_color="transparent")
        self.delete_csv_tile.pack(anchor="w", fill="x", padx=16, pady=(6, 14))
        self.delete_csv_toggle = self.make_option_check(self.delete_csv_tile, "Delete CSV after Excel is created", self.delete_csv_var)
        self.delete_csv_toggle.pack(anchor="w", fill="x")

        agency_card = ctk.CTkFrame(page, fg_color=themed_color("hero_card_bg"), corner_radius=22)
        agency_card.grid(row=5, column=0, columnspan=2, sticky="ew", padx=28, pady=(18, 0))
        agency_card.grid_columnconfigure((0, 1, 2), weight=1)
        ctk.CTkLabel(
            agency_card,
            text="Agency Content List",
            font=ctk.CTkFont(size=20, weight="bold"),
            text_color=themed_color("body_fg"),
        ).grid(row=0, column=0, columnspan=3, sticky="w", padx=24, pady=(20, 8))
        ctk.CTkLabel(
            agency_card,
            text="Use the agency transfer headers. RC Series is required; other values can be filled once for the whole sheet.",
            font=ctk.CTkFont(size=13),
            text_color=themed_color("hint_fg"),
            wraplength=920,
            justify="left",
        ).grid(row=1, column=0, columnspan=3, sticky="w", padx=24, pady=(0, 10))
        self.agency_template_toggle = self.make_option_check(agency_card, "Use agency content-list headers", self.agency_template_var, command=self.sync_agency_template_state)
        self.agency_template_toggle.grid(row=2, column=0, columnspan=3, sticky="w", padx=24, pady=(0, 8))

        agency_fields = [
            ("RG", "rg"),
            ("SG", "sg"),
            ("Series", "series"),
            ("RC Series", "rc_series"),
            ("Department / Organization", "dept_organization"),
            ("RC Series Name", "rc_series_name"),
            ("Division", "division"),
            ("Section", "section"),
            ("Unit", "unit"),
            ("Begin Date", "begin_date"),
            ("End Date", "end_date"),
            ("Material Type", "material_type"),
            ("Confidential", "confidential"),
            ("Disposition Date", "disposition_date"),
            ("TD Number", "td_num"),
            ("Box Number", "box_num"),
            ("Location ID", "location_id"),
            ("Record Level", "record_level"),
            ("Description", "description"),
            ("Location Override", "location"),
            ("Comments", "comments"),
        ]
        for index, (label, key) in enumerate(agency_fields):
            row = 3 + index // 3
            col = index % 3
            field = ctk.CTkFrame(agency_card, fg_color="transparent")
            field.grid(row=row, column=col, sticky="ew", padx=(24 if col == 0 else 8, 24 if col == 2 else 8), pady=(0, 10))
            field.grid_columnconfigure(0, weight=1)
            ctk.CTkLabel(field, text=label, font=ctk.CTkFont(size=12, weight="bold"), text_color=themed_color("hint_fg")).grid(row=0, column=0, sticky="w", pady=(0, 4))
            ctk.CTkEntry(field, textvariable=self.agency_field_vars[key], font=ctk.CTkFont(size=13)).grid(row=1, column=0, sticky="ew")

        actions = ctk.CTkFrame(page, fg_color="transparent")
        actions.grid(row=6, column=0, columnspan=2, sticky="ew", padx=28, pady=(18, 0))
        self.reset_button = self.make_secondary_button(actions, "Reset", self.reset_fields)
        self.reset_button.pack(side="left")
        self.generate_button = self.make_primary_button(actions, "Generate", self.start_scan)
        self.generate_button.pack(side="left", padx=(10, 0))
        self.stop_button = self.make_secondary_button(actions, "Stop Scan", self.stop_scan)
        self.stop_button.pack(side="left", padx=(10, 0))
        self.stop_button.configure(state="disabled")
        self.open_folder_button = self.make_secondary_button(actions, "Output Folder", self.open_output_folder)
        self.open_folder_button.pack(side="left", padx=(10, 0))

        progress_card = ctk.CTkFrame(page, fg_color=themed_color("card_bg"), corner_radius=22)
        progress_card.grid(row=7, column=0, columnspan=2, sticky="ew", padx=28, pady=(18, 0))
        progress_card.grid_columnconfigure((0, 1, 2), weight=1)
        ctk.CTkLabel(progress_card, text="Progress", font=ctk.CTkFont(size=20, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", padx=24, pady=(20, 10))
        metric_row = ctk.CTkFrame(progress_card, fg_color="transparent")
        metric_row.grid(row=1, column=0, columnspan=3, sticky="ew", padx=24, pady=(0, 14))
        metric_row.grid_columnconfigure((0, 1, 2), weight=1)
        self.make_metric_card(metric_row, "Files Included", self.scan_files_var, accent=True).grid(row=0, column=0, sticky="ew", padx=(0, 8))
        self.make_metric_card(metric_row, "Items Skipped", self.scan_skipped_var).grid(row=0, column=1, sticky="ew", padx=4)
        self.make_metric_card(metric_row, "Saved Output", self.scan_saved_var).grid(row=0, column=2, sticky="ew", padx=(8, 0))

        self.progress = ctk.CTkProgressBar(progress_card, progress_color=themed_color("progress_fill"), mode="determinate")
        self.progress.grid(row=2, column=0, columnspan=3, sticky="ew", padx=24)
        self.progress.set(0)
        ctk.CTkLabel(progress_card, textvariable=self.status_var, text_color=themed_color("body_fg"), wraplength=940, justify="left").grid(row=3, column=0, columnspan=3, sticky="w", padx=24, pady=(12, 18))

        summary_card = ctk.CTkFrame(page, fg_color=themed_color("card_bg"), corner_radius=22)
        summary_card.grid(row=7, column=0, columnspan=2, sticky="nsew", padx=28, pady=(18, 28))
        summary_card.grid_columnconfigure(0, weight=1)
        ctk.CTkLabel(summary_card, text="Summary", font=ctk.CTkFont(size=20, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", padx=24, pady=(20, 10))
        self.summary = ctk.CTkTextbox(
            summary_card,
            height=280,
            wrap="word",
            fg_color=themed_color("card_alt_bg"),
            text_color=themed_color("body_fg"),
            border_width=0,
            font=("Menlo", 11),
        )
        self.summary.grid(row=1, column=0, sticky="nsew", padx=24, pady=(0, 24))
        self.sync_clone_state()
        return page

    def build_email_page(self, parent) -> ctk.CTkScrollableFrame:
        self.email_page = EmailCopyPage(self, parent)
        return self.email_page.page

    def build_about_page(self, parent) -> ctk.CTkScrollableFrame:
        page = self.build_scrollable_root(parent)
        page.pack_forget()
        page.grid_columnconfigure(0, weight=1)

        card = ctk.CTkFrame(page, fg_color=themed_color("card_bg"), corner_radius=24)
        card.grid(row=0, column=0, sticky="ew", padx=28, pady=28)
        ctk.CTkLabel(card, text="About Content List Toolkit", font=ctk.CTkFont(size=32, weight="bold"), text_color=themed_color("title_fg")).pack(anchor="w", padx=28, pady=(26, 0))
        ctk.CTkLabel(
            card,
            text=(
                "Content List Toolkit helps you create a simple file list from a folder and "
                "copy supported email files into a new location."
            ),
            font=ctk.CTkFont(size=14),
            text_color=themed_color("hint_fg"),
            wraplength=920,
            justify="left",
        ).pack(anchor="w", padx=28, pady=(10, 20))
        details = ctk.CTkFrame(card, fg_color=themed_color("card_alt_bg"), corner_radius=18)
        details.pack(fill="x", padx=28, pady=(0, 18))
        ctk.CTkLabel(details, text="Written by Bryan Snyder", font=ctk.CTkFont(size=16, weight="bold"), text_color=themed_color("body_fg")).pack(anchor="w", padx=20, pady=(18, 6))
        ctk.CTkLabel(details, text=f"GitHub: {PLACEHOLDER_GITHUB_URL}", text_color=themed_color("body_fg"), wraplength=860, justify="left").pack(anchor="w", padx=20)
        self.make_secondary_button(details, "Open GitHub Link", lambda: webbrowser.open_new_tab(PLACEHOLDER_GITHUB_URL)).pack(anchor="w", padx=20, pady=(12, 18))

        open_source = ctk.CTkFrame(card, fg_color=themed_color("card_alt_bg"), corner_radius=18)
        open_source.pack(fill="x", padx=28, pady=(0, 28))
        ctk.CTkLabel(open_source, text="Open source note", font=ctk.CTkFont(size=18, weight="bold"), text_color=themed_color("body_fg")).pack(anchor="w", padx=20, pady=(18, 8))
        ctk.CTkLabel(
            open_source,
            text=(
                "This project is being prepared for an open source release.\n"
                "TODO: decide the final attribution requirement before publishing."
            ),
            text_color=themed_color("body_fg"),
            wraplength=860,
            justify="left",
        ).pack(anchor="w", padx=20, pady=(0, 18))
        return page

    def focus_source_entry(self) -> None:
        if self.source_entry is not None and self.source_entry.winfo_exists():
            self.source_entry.focus_force()
            self.source_entry.icursor("end")

    def make_primary_button(self, parent, text: str, command) -> ctk.CTkButton:
        return ctk.CTkButton(
            parent,
            text=text,
            command=command,
            fg_color=themed_color("primary_bg"),
            hover_color=themed_color("progress_fill"),
            text_color=themed_color("primary_fg"),
            corner_radius=12,
            height=44,
            font=ctk.CTkFont(size=14, weight="bold"),
        )

    def make_secondary_button(self, parent, text: str, command) -> ctk.CTkButton:
        return ctk.CTkButton(
            parent,
            text=text,
            command=command,
            fg_color=themed_color("secondary_bg"),
            hover_color=themed_color("secondary_hover"),
            text_color=themed_color("secondary_fg"),
            border_color=themed_color("secondary_bg"),
            border_width=0,
            corner_radius=12,
            height=44,
            font=ctk.CTkFont(size=13, weight="bold"),
        )

    def make_nav_button(self, parent, text: str, command) -> ctk.CTkButton:
        return ctk.CTkButton(
            parent,
            text=text,
            command=command,
            anchor="w",
            height=46,
            corner_radius=14,
            fg_color="transparent",
            hover_color=themed_color("card_bg"),
            text_color=themed_color("sidebar_idle_fg"),
            font=ctk.CTkFont(size=15, weight="bold"),
        )

    def make_metric_card(self, parent, title: str, value_var: tk.StringVar, accent: bool = False) -> ctk.CTkFrame:
        frame = ctk.CTkFrame(parent, fg_color=themed_color("card_alt_bg"), corner_radius=18)
        ctk.CTkLabel(
            frame,
            text=title,
            font=ctk.CTkFont(size=11, weight="bold"),
            text_color=themed_color("hint_fg"),
            anchor="w",
            justify="left",
            wraplength=130,
        ).pack(fill="x", padx=16, pady=(14, 6))
        ctk.CTkLabel(
            frame,
            textvariable=value_var,
            font=ctk.CTkFont(size=22, weight="bold"),
            text_color=themed_color("progress_fill" if accent else "body_fg"),
            anchor="w",
            justify="left",
        ).pack(fill="x", padx=16, pady=(0, 14))
        return frame

    def make_option_check(self, parent, text: str, variable: tk.BooleanVar, command=None) -> ctk.CTkCheckBox:
        return ctk.CTkCheckBox(
            parent,
            text=text,
            variable=variable,
            command=command,
            text_color=themed_color("body_fg"),
            fg_color=themed_color("primary_bg"),
            hover_color=themed_color("primary_hover"),
            border_color=themed_color("border"),
            checkmark_color=themed_color("primary_fg"),
            corner_radius=8,
        )

    def make_select_tile(self, parent, text: str, variable: tk.StringVar, values: list[str]) -> ctk.CTkFrame:
        tile = ctk.CTkFrame(parent, fg_color=themed_color("card_bg"), corner_radius=18)
        ctk.CTkLabel(
            tile,
            text=text,
            text_color=themed_color("body_fg"),
            font=ctk.CTkFont(size=13, weight="bold"),
        ).pack(anchor="w", padx=16, pady=(16, 8))
        menu = ctk.CTkOptionMenu(
            tile,
            values=values,
            variable=variable,
            fg_color=themed_color("secondary_bg"),
            button_color=themed_color("primary_bg"),
            button_hover_color=themed_color("primary_hover"),
            text_color=themed_color("body_fg"),
            dropdown_fg_color=themed_color("card_bg"),
            dropdown_hover_color=themed_color("secondary_hover"),
            dropdown_text_color=themed_color("body_fg"),
            corner_radius=10,
        )
        menu.pack(fill="x", padx=16, pady=(0, 16))
        tile.menu = menu
        return tile

    def make_field_card(self, parent, label: str, variable: tk.StringVar, command, hint: str = "") -> ctk.CTkFrame:
        card = ctk.CTkFrame(parent, fg_color=themed_color("card_bg"), corner_radius=22)
        card.grid_columnconfigure(0, weight=1)
        ctk.CTkLabel(card, text=label, font=ctk.CTkFont(size=18, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", padx=22, pady=(20, 6))
        if hint:
            ctk.CTkLabel(card, text=hint, font=ctk.CTkFont(size=12), text_color=themed_color("hint_fg"), wraplength=420, justify="left").grid(row=1, column=0, sticky="w", padx=22, pady=(0, 12))
        row_idx = 2 if hint else 1
        entry_wrap = ctk.CTkFrame(card, fg_color="transparent")
        entry_wrap.grid(row=row_idx, column=0, sticky="ew", padx=22, pady=(0, 20))
        entry_wrap.grid_columnconfigure(0, weight=1)
        entry = ctk.CTkEntry(
            entry_wrap,
            textvariable=variable,
            fg_color=themed_color("entry_bg"),
            text_color=themed_color("entry_fg"),
            border_color=themed_color("border"),
            height=44,
        )
        entry.grid(row=0, column=0, sticky="ew")
        self.make_secondary_button(entry_wrap, "Browse", command).grid(row=0, column=1, padx=(12, 0))
        card.entry = entry
        return card

    def make_text_field_card(self, parent, label: str, variable: tk.StringVar, hint: str = "") -> ctk.CTkFrame:
        card = ctk.CTkFrame(parent, fg_color=themed_color("card_alt_bg"), corner_radius=18)
        card.grid_columnconfigure(0, weight=1)
        ctk.CTkLabel(card, text=label, font=ctk.CTkFont(size=15, weight="bold"), text_color=themed_color("body_fg")).grid(row=0, column=0, sticky="w", padx=18, pady=(18, 6))
        entry = ctk.CTkEntry(
            card,
            textvariable=variable,
            fg_color=themed_color("entry_bg"),
            text_color=themed_color("entry_fg"),
            border_color=themed_color("border"),
            height=42,
        )
        entry.grid(row=1, column=0, sticky="ew", padx=18)
        if hint:
            ctk.CTkLabel(card, text=hint, font=ctk.CTkFont(size=12), text_color=themed_color("hint_fg"), wraplength=420, justify="left").grid(row=2, column=0, sticky="w", padx=18, pady=(8, 18))
        else:
            ctk.CTkFrame(card, fg_color="transparent", height=18).grid(row=2, column=0)
        card.entry = entry
        return card

    def toggle_theme(self) -> None:
        selected_mode = theme_mode_from_label(self.theme_mode_var.get())
        save_theme_mode(selected_mode)
        ctk.set_appearance_mode(theme_mode_label(selected_mode))
        self.colors = palette_for_mode(effective_theme_mode(selected_mode))
        self.root.configure(fg_color=themed_color("app_bg"))
        self.configure_style()
        self.show_page(self.active_page)

    def show_page(self, page: str) -> None:
        self.active_page = page
        for name, frame in self.page_frames.items():
            if name == page:
                frame.pack(fill="both", expand=True)
            else:
                frame.pack_forget()
        for name, button in self.nav_buttons.items():
            if name == page:
                button.configure(
                    fg_color=themed_color("sidebar_active_bg"),
                    hover_color=themed_color("sidebar_active_bg"),
                    text_color=themed_color("sidebar_active_fg"),
                )
            else:
                button.configure(
                    fg_color="transparent",
                    hover_color=themed_color("card_bg"),
                    text_color=themed_color("sidebar_idle_fg"),
                )
        if page == "content":
            self.root.after(50, self.focus_source_entry)

    def sync_xlsx_state(self) -> None:
        state = "normal" if self.xlsx_var.get() else "disabled"
        if self.preserve_zeros_toggle is not None:
            self.preserve_zeros_toggle.configure(state=state)
        if self.delete_csv_toggle is not None:
            self.delete_csv_toggle.configure(state=state)
        if self.delete_csv_tile is not None:
            if self.xlsx_var.get():
                if not self.delete_csv_tile.winfo_manager():
                    self.delete_csv_tile.pack(anchor="w", fill="x", padx=16, pady=(6, 14))
            else:
                self.delete_csv_tile.pack_forget()
        if self.xlsx_var.get():
            if not self.preserve_zeros_var.get():
                self.preserve_zeros_var.set(True)
            if not self.delete_csv_var.get():
                self.delete_csv_var.set(True)
        else:
            self.preserve_zeros_var.set(False)
            self.delete_csv_var.set(False)

    def sync_folders_only_state(self) -> None:
        folders_only = self.folders_only_var.get()
        xlsx_state = "disabled" if folders_only else "normal"
        hash_state = "disabled" if folders_only else "normal"
        if self.hash_select_tile is not None and hasattr(self.hash_select_tile, "menu"):
            if not self.clone_verify_var.get():
                self.hash_select_tile.menu.configure(state=hash_state)
        if self.preserve_zeros_toggle is not None:
            self.preserve_zeros_toggle.configure(state=xlsx_state)
        if self.delete_csv_toggle is not None:
            self.delete_csv_toggle.configure(state=xlsx_state)
        if folders_only:
            self.xlsx_var.set(False)
            self.preserve_zeros_var.set(False)
            self.delete_csv_var.set(False)
            if self.delete_csv_tile is not None:
                self.delete_csv_tile.pack_forget()
            if hasattr(self, "folder_depth_frame") and self.folder_depth_frame is not None:
                self.folder_depth_frame.pack(anchor="w", fill="x", padx=32, pady=(0, 6))
            source = self.source_var.get().strip()
            if source:
                self.output_name_var.set(default_folder_list_output_name(Path(source)))
        else:
            self.xlsx_var.set(True)
            self.preserve_zeros_var.set(True)
            self.delete_csv_var.set(True)
            if hasattr(self, "folder_depth_frame") and self.folder_depth_frame is not None:
                self.folder_depth_frame.pack_forget()
            self.sync_xlsx_state()
            source = self.source_var.get().strip()
            if source:
                self.output_name_var.set(default_output_name(Path(source)))

    def sync_agency_template_state(self) -> None:
        if self.agency_template_var.get() and self.folders_only_var.get():
            self.folders_only_var.set(False)
            self.sync_folders_only_state()

    def agency_fields_payload(self) -> dict[str, str]:
        return {key: value.get().strip() for key, value in self.agency_field_vars.items()}

    def clone_hash_algorithm(self) -> str:
        if is_blake3_available():
            return HASH_ALGORITHM_BLAKE3
        return HASH_ALGORITHM_SHA1

    def sync_clone_state(self) -> None:
        if self.clone_verify_var.get():
            algorithm = self.clone_hash_algorithm()
            self.hash_algorithm_var.set(hash_algorithm_label(algorithm))
            if self.hash_select_tile is not None and hasattr(self.hash_select_tile, "menu"):
                self.hash_select_tile.menu.configure(state="disabled")
            note = "Clone verification uses the fastest full-file hash automatically: BLAKE3."
            if algorithm == HASH_ALGORITHM_SHA1:
                note = "BLAKE3 is not installed, so clone verification will use SHA-1 for this run and the app will tell you before scanning."
            self.clone_note_var.set(note)
            if self.generate_button is not None:
                self.generate_button.configure(text="Verify")
            return
        if self.hash_select_tile is not None and hasattr(self.hash_select_tile, "menu"):
            self.hash_select_tile.menu.configure(state="normal")
        self.clone_note_var.set("Turn this on to scan the 1st Drive first, then choose the 2nd Drive after the 1st Drive finishes.")
        if self.generate_button is not None:
            self.generate_button.configure(text="Generate")

    def sync_action_buttons(self) -> None:
        running_state = "disabled" if self.running else "normal"
        if self.generate_button is not None:
            self.generate_button.configure(state=running_state)
        if self.stop_button is not None:
            self.stop_button.configure(state="normal" if self.running else "disabled")
        if self.open_folder_button is not None:
            self.open_folder_button.configure(state="normal")
        if self.reset_button is not None:
            self.reset_button.configure(state=running_state)
        if self.about_button is not None:
            self.about_button.configure(state="normal")

    def choose_source(self) -> None:
        chosen = choose_directory(self.root, "Choose Source Folder", self.source_var.get(), True, self.colors)
        if chosen:
            self.source_var.set(chosen)
            try:
                if Path(self.output_dir_var.get()).expanduser().resolve() == Path(chosen).expanduser().resolve():
                    self.output_dir_var.set(str(default_scan_output_dir(Path(chosen))))
            except OSError:
                pass
            if not self.output_name_var.get().strip():
                self.output_name_var.set(default_output_name(Path(chosen)))

    def choose_output(self) -> None:
        chosen = choose_directory(self.root, "Choose Output Folder", self.output_dir_var.get(), False, self.colors)
        if chosen:
            source = Path(self.source_var.get()).expanduser().resolve()
            output = Path(chosen).expanduser().resolve()
            if output == source:
                messagebox.showerror(
                    "Invalid output folder",
                    "Output folder cannot be the same as the source folder.",
                )
                return
            self.output_dir_var.set(chosen)

    def reset_fields(self) -> None:
        cwd = Path(os.getcwd())
        self.source_var.set(str(cwd))
        self.output_dir_var.set(str(default_scan_output_dir(cwd)))
        self.output_name_var.set(default_output_name(cwd))
        self.exclude_var.set("")
        self.hash_algorithm_var.set(hash_algorithm_label(default_hash_algorithm()))
        loaded_mode = load_theme_mode()
        self.theme_mode_var.set(theme_mode_label(loaded_mode))
        ctk.set_appearance_mode(theme_mode_label(loaded_mode))
        self.colors = palette_for_mode(effective_theme_mode(loaded_mode))
        self.clone_verify_var.set(False)
        self.folders_only_var.set(False)
        self.folder_depth_var.set("0")
        self.hidden_var.set(True)
        self.system_var.set(True)
        self.xlsx_var.set(True)
        self.preserve_zeros_var.set(True)
        self.delete_csv_var.set(True)
        self.agency_template_var.set(False)
        for key, value in self.agency_field_vars.items():
            if key == "material_type":
                value.set("Born Digital")
            elif key == "record_level":
                value.set("Item")
            else:
                value.set("")
        if self.progress is not None:
            self.progress.set(0)
        self.status_var.set("Choose a folder to scan, then click Generate.")
        self.scan_files_var.set("0")
        self.scan_skipped_var.set("0")
        self.scan_saved_var.set("Waiting")
        self.append_summary("")
        self.sync_folders_only_state()
        self.sync_agency_template_state()
        self.sync_xlsx_state()
        self.sync_clone_state()
        self.root.configure(fg_color=themed_color("app_bg"))
        self.configure_style()
        self.show_page("content")
        self.focus_source_entry()

    def show_about_dialog(self) -> None:
        self.show_page("about")

    def open_email_copy_window(self) -> None:
        if self.running:
            return
        self.show_page("email")
        email_page = getattr(self, "email_page", None)
        if email_page is not None:
            self.root.after(50, email_page.focus_source_entry)

    def open_output_folder(self) -> None:
        open_in_file_manager(Path(self.output_dir_var.get() or os.getcwd()))

    def append_summary(self, text: str) -> None:
        self.summary.configure(state="normal")
        self.summary.delete("1.0", "end")
        self.summary.insert("end", text)
        self.summary.configure(state="disabled")

    def selected_scan_hash(self) -> str:
        if self.clone_verify_var.get():
            return self.clone_hash_algorithm()
        return normalize_hash_algorithm(self.hash_algorithm_var.get())

    def begin_scan_run(self) -> None:
        self.running = True
        self.scan_cancel_event = threading.Event()
        self.pending_clone_result = None
        self.sync_action_buttons()
        if self.progress is not None:
            self.progress.stop()
            self.progress.configure(mode="determinate")
            self.progress.set(0)
        self.status_var.set("Getting everything ready...")
        self.scan_files_var.set("0")
        self.scan_skipped_var.set("0")
        self.scan_saved_var.set("Working")
        self.show_page("content")

    def launch_scan_thread(self, payload: dict[str, object]) -> None:
        thread = threading.Thread(
            target=self.run_scan_thread,
            args=(payload,),
            daemon=True,
        )
        thread.start()

    def prompt_for_clone_drive_b(self, source_dir: Path) -> Path | None:
        self.status_var.set("1st Drive completed. Choose whether to eject it before continuing to the Clone/2nd Drive.")
        wants_eject = messagebox.askyesno(
            "1st Drive Completed",
            "1st Drive completed.\n\nDo you want to eject it before you continue to the Clone/2nd Drive?",
        )
        if wants_eject:
            ejected, message = eject_drive_or_folder(source_dir)
            if ejected:
                messagebox.showinfo("1st Drive Ejected", message)
                self.status_var.set("1st Drive was ejected. Choose the Clone/2nd Drive next.")
            else:
                messagebox.showinfo("Eject Not Completed", message + "\n\nChoose the Clone/2nd Drive when you are ready.")
                self.status_var.set("Choose the Clone/2nd Drive.")
        else:
            self.status_var.set("Choose the Clone/2nd Drive.")
        while True:
            chosen = choose_directory(
                self.root,
                "Choose Clone/2nd Drive",
                str(source_dir.parent),
                True,
                self.colors,
            )
            if not chosen:
                return None
            drive_b = Path(chosen).expanduser().resolve()
            if not drive_b.is_dir():
                messagebox.showerror("Invalid Clone/2nd Drive", f"The Clone/2nd Drive does not exist:\n{drive_b}")
                continue
            if drive_b == source_dir:
                messagebox.showerror("Invalid Clone/2nd Drive", "The Clone/2nd Drive must be different from the 1st Drive.")
                continue
            return drive_b

    def start_scan(self) -> None:
        if self.running:
            return

        source_dir = Path(self.source_var.get()).expanduser().resolve()
        output_dir = Path(self.output_dir_var.get()).expanduser().resolve()
        output_name = self.output_name_var.get().strip() or default_output_name(source_dir)
        excluded_exts = normalize_exts(self.exclude_var.get())

        if not source_dir.is_dir():
            messagebox.showerror("Invalid source folder", f"Source folder does not exist:\n{source_dir}")
            return
        if output_dir == source_dir:
            messagebox.showerror(
                "Invalid output folder",
                "Output folder cannot be the same as the source folder.",
            )
            return
        if not output_name.lower().endswith(".csv"):
            messagebox.showerror("Invalid output file", "Output file name must end in .csv")
            return
        if self.agency_template_var.get() and self.clone_verify_var.get():
            messagebox.showerror("Agency template", "Agency template output cannot be used with Clone Drive Verification.")
            return
        agency_fields = self.agency_fields_payload()
        if self.agency_template_var.get() and not agency_fields["rc_series"]:
            messagebox.showerror("Agency template", "RC Series is required for agency content-list output.")
            return
        if self.agency_template_var.get():
            try:
                agency_fields = normalize_agency_template_fields(agency_fields)
            except ValueError as exc:
                messagebox.showerror("Agency template", str(exc))
                return

        output_path = output_dir / output_name
        first_csv_output_path = csv_output_path_for_part(output_path, 1)
        if first_csv_output_path.exists():
            confirmed = messagebox.askyesno("Overwrite file?", f"{first_csv_output_path}\n\nalready exists. Overwrite it?")
            if not confirmed:
                return

        selected_hash = self.selected_scan_hash()
        if self.clone_verify_var.get() and selected_hash == HASH_ALGORITHM_SHA1 and not is_blake3_available():
            messagebox.showinfo(
                "Clone verification hash",
                "BLAKE3 is not installed, so Verify Clones will use SHA-1 for this run.",
            )
        elif selected_hash == "blake3" and not is_blake3_available():
            messagebox.showerror(
                "BLAKE3 not installed",
                "BLAKE3 was selected, but the Python 'blake3' package is not installed.\n\nInstall it with: pip install -r requirements.txt",
            )
            return

        self.begin_scan_run()
        if self.clone_verify_var.get():
            self.append_summary("Preparing the 1st Drive for clone verification...")
            self.launch_scan_thread(
                {
                    "mode": "clone-drive-a",
                    "source_dir": source_dir,
                    "output_path": output_path,
                    "excluded_exts": excluded_exts,
                    "hash_algorithm": selected_hash,
                    "create_xlsx": self.xlsx_var.get(),
                    "preserve_zeros": self.preserve_zeros_var.get(),
                    "delete_csv_requested": self.delete_csv_var.get(),
                    "agency_template": False,
                    "agency_fields": {},
                }
            )
            return

        self.append_summary("Preparing your file list...")
        self.launch_scan_thread(
            {
                "mode": "scan",
                "source_dir": source_dir,
                "output_path": output_path,
                "excluded_exts": excluded_exts,
                "hash_algorithm": selected_hash,
                "create_xlsx": self.xlsx_var.get(),
                "preserve_zeros": self.preserve_zeros_var.get(),
                "delete_csv_requested": self.delete_csv_var.get(),
                "agency_template": self.agency_template_var.get(),
                "agency_fields": agency_fields,
            }
        )

    def stop_scan(self) -> None:
        if self.scan_cancel_event is None:
            return
        self.scan_cancel_event.set()
        self.status_var.set("Stopping scan...")

    def run_scan_thread(self, payload: dict[str, object]) -> None:
        try:
            self.message_queue.put(("status", "Counting files and folders..."))
            source_dir = payload["source_dir"]
            output_path = payload["output_path"]
            excluded_exts = payload["excluded_exts"]
            selected_hash = payload["hash_algorithm"]
            create_xlsx = payload["create_xlsx"]
            preserve_zeros = payload["preserve_zeros"]
            delete_csv_requested = payload["delete_csv_requested"]
            mode = payload["mode"]
            agency_template = bool(payload.get("agency_template", False))
            agency_fields = payload.get("agency_fields") or {}

            def on_progress(progress: ScanProgress) -> None:
                self.message_queue.put(("progress", progress))

            result = run_scan(
                source_dir,
                output_path,
                hash_algorithm=selected_hash,
                include_hidden=not self.hidden_var.get(),
                include_system=not self.system_var.get(),
                excluded_exts=excluded_exts,
                create_xlsx=create_xlsx,
                preserve_zeros=preserve_zeros,
                delete_csv=False if mode != "scan" and delete_csv_requested else delete_csv_requested,
                max_rows_per_csv=DEFAULT_MAX_ROWS_PER_CSV,
                folders_only=self.folders_only_var.get(),
                folder_depth=max(0, int(self.folder_depth_var.get() or "0")),
                agency_template=agency_template,
                agency_fields=agency_fields,
                progress_callback=on_progress,
                cancel_event=self.scan_cancel_event,
            )

            if mode == "clone-drive-a":
                self.message_queue.put(
                    (
                        "clone_drive_a_done",
                        {
                            "drive_a_result": result,
                            "source_dir": source_dir,
                            "output_path": output_path,
                            "excluded_exts": excluded_exts,
                            "hash_algorithm": selected_hash,
                            "create_xlsx": create_xlsx,
                            "preserve_zeros": preserve_zeros,
                            "delete_csv_requested": delete_csv_requested,
                            "agency_template": False,
                            "agency_fields": {},
                        },
                    )
                )
                return

            if mode == "clone-drive-b":
                drive_a_result = payload["drive_a_result"]
                diff_path = clone_diff_csv_path(payload["base_output_path"])
                report_path = clone_diff_report_path(payload["base_output_path"])

                def on_compare(progress: CloneCompareProgress) -> None:
                    self.message_queue.put(("clone_progress", progress))

                clone_result = compare_scan_outputs(
                    drive_a_result,
                    result,
                    diff_path,
                    report_path,
                    progress_callback=on_compare,
                    cancel_event=self.scan_cancel_event,
                )
                delete_deferred_scan_csvs(drive_a_result, delete_csv_requested)
                delete_deferred_scan_csvs(result, delete_csv_requested)
                self.message_queue.put(("clone_done", clone_result))
                return

            self.message_queue.put(("done", result))
        except ScanCanceled:
            if payload["mode"] == "clone-drive-b":
                self.message_queue.put(("clone_canceled", payload.get("drive_a_result")))
                return
            self.message_queue.put(("canceled", None))
        except Exception as exc:  # pragma: no cover
            self.message_queue.put(("error", str(exc)))

    def pump_queue(self) -> None:
        try:
            while True:
                kind, payload = self.message_queue.get_nowait()
                if kind == "status":
                    self.status_var.set(str(payload))
                elif kind == "progress":
                    progress: ScanProgress = payload
                    if progress.phase == "counting":
                        if self.progress is not None:
                            self.progress.stop()
                            self.progress.configure(mode="determinate")
                            self.progress.set(0)
                        self.status_var.set(
                            f"Counting... {progress.files} files in {progress.directories} folders so far ({human_bytes(progress.bytes)} found)."
                        )
                    else:
                        if self.progress is not None:
                            self.progress.stop()
                            self.progress.configure(mode="determinate")
                            self.progress.set(progress.bytes / max(1, progress.total_bytes))
                        self.status_var.set(
                            f"Scanning... {progress.files} of {progress.total_files} files, {human_bytes(progress.bytes)} of {human_bytes(progress.total_bytes)}: {progress.current_name}"
                        )
                elif kind == "clone_progress":
                    progress: CloneCompareProgress = payload
                    if self.progress is not None:
                        self.progress.stop()
                        self.progress.configure(mode="determinate")
                        self.progress.set(compare_progress_fraction(progress))
                    self.status_var.set(
                        f"Comparing the 1st Drive and 2nd Drive... {progress.compared} items checked, {progress.differences} differences found: {progress.current_name}"
                    )
                elif kind == "clone_drive_a_done":
                    payload_map = payload
                    drive_a_result = payload_map["drive_a_result"]
                    self.status_var.set("The 1st Drive content list is ready. Continue to the Clone/2nd Drive next.")
                    self.scan_files_var.set(str(drive_a_result.files))
                    self.scan_skipped_var.set(str(drive_a_result.filtered))
                    self.scan_saved_var.set("1st Drive Saved")
                    self.append_summary(build_scan_summary(drive_a_result))
                    if self.progress is not None:
                        self.progress.stop()
                        self.progress.configure(mode="determinate")
                        self.progress.set(1)

                    drive_b_dir = self.prompt_for_clone_drive_b(payload_map["source_dir"])
                    if drive_b_dir is None:
                        self.running = False
                        self.scan_cancel_event = None
                        self.status_var.set("The Clone/2nd Drive was not chosen. The 1st Drive content list remains saved.")
                        self.scan_saved_var.set("1st Drive Only")
                        self.sync_action_buttons()
                        continue

                    if self.progress is not None:
                        self.progress.stop()
                        self.progress.configure(mode="determinate")
                        self.progress.set(0)
                    self.status_var.set("Counting files and folders for the 2nd Drive...")
                    self.scan_saved_var.set("Comparing")
                    self.append_summary("The 1st Drive content list is saved. Scanning the 2nd Drive next...")
                    self.launch_scan_thread(
                        {
                            "mode": "clone-drive-b",
                            "source_dir": drive_b_dir,
                            "output_path": clone_output_path_for_drive_b(payload_map["output_path"]),
                            "base_output_path": payload_map["output_path"],
                            "excluded_exts": payload_map["excluded_exts"],
                            "hash_algorithm": payload_map["hash_algorithm"],
                            "create_xlsx": payload_map["create_xlsx"],
                            "preserve_zeros": payload_map["preserve_zeros"],
                            "delete_csv_requested": payload_map["delete_csv_requested"],
                            "agency_template": False,
                            "agency_fields": {},
                            "drive_a_result": drive_a_result,
                        }
                    )
                elif kind == "done":
                    self.running = False
                    self.scan_cancel_event = None
                    result = payload
                    self.status_var.set(f"Your file list is ready. {result.files} files were included.")
                    self.scan_files_var.set(str(result.files))
                    self.scan_skipped_var.set(str(result.filtered))
                    if result.xlsx_path and result.csv_deleted:
                        self.scan_saved_var.set("Report + Excel (CSV removed)")
                    elif result.xlsx_path:
                        if result.xlsx_parts > 1:
                            self.scan_saved_var.set(f"CSV + Report + {result.xlsx_parts} Excel files")
                        else:
                            self.scan_saved_var.set("CSV + Report + Excel")
                    else:
                        self.scan_saved_var.set("CSV + Report")
                    self.append_summary(build_scan_summary(result))
                    if self.progress is not None:
                        self.progress.stop()
                        self.progress.configure(mode="determinate")
                        self.progress.set(1)
                    self.sync_action_buttons()
                elif kind == "clone_done":
                    self.running = False
                    self.scan_cancel_event = None
                    result: CloneVerificationResult = payload
                    self.pending_clone_result = result
                    self.status_var.set(
                        f"Clone verification finished. Verdict: {result.verdict}."
                    )
                    self.scan_files_var.set(f"1st: {result.drive_a.files}  2nd: {result.drive_b.files}")
                    self.scan_skipped_var.set(f"1st: {result.drive_a.filtered}  2nd: {result.drive_b.filtered}")
                    self.scan_saved_var.set("2 Lists + Diff Report")
                    self.append_summary(build_clone_verification_summary(result))
                    if self.progress is not None:
                        self.progress.stop()
                        self.progress.configure(mode="determinate")
                        self.progress.set(1)
                    self.sync_action_buttons()
                elif kind == "canceled":
                    self.running = False
                    self.scan_cancel_event = None
                    self.status_var.set("Scan stopped. Partial output was removed.")
                    self.scan_saved_var.set("Stopped")
                    self.append_summary("Scan stopped before completion.")
                    if self.progress is not None:
                        self.progress.stop()
                        self.progress.configure(mode="determinate")
                        self.progress.set(0)
                    self.sync_action_buttons()
                elif kind == "clone_canceled":
                    self.running = False
                    self.scan_cancel_event = None
                    drive_a_result = payload
                    self.status_var.set("Clone verification stopped. The 1st Drive remains saved and partial 2nd Drive output was removed.")
                    self.scan_saved_var.set("1st Drive Saved")
                    if drive_a_result is not None:
                        self.scan_files_var.set(str(drive_a_result.files))
                        self.scan_skipped_var.set(str(drive_a_result.filtered))
                        self.append_summary(build_scan_summary(drive_a_result))
                    else:
                        self.append_summary("Clone verification stopped before the 2nd Drive finished.")
                    if self.progress is not None:
                        self.progress.stop()
                        self.progress.configure(mode="determinate")
                        self.progress.set(0)
                    self.sync_action_buttons()
                elif kind == "error":
                    self.running = False
                    self.scan_cancel_event = None
                    self.status_var.set("Something went wrong while making the file list.")
                    self.sync_action_buttons()
                    if self.progress is not None:
                        self.progress.stop()
                        self.progress.configure(mode="determinate")
                    messagebox.showerror("Scan failed", str(payload))
        except queue.Empty:
            pass
        self.root.after(100, self.pump_queue)

    def run(self) -> int:
        self.root.mainloop()
        return 0


def main() -> int:
    args = parse_args()
    has_explicit_cli_args = any(arg != "--cli" for arg in sys.argv[1:])

    if not args.cli and not has_explicit_cli_args:
        if tk is None:
            print("Tkinter is not available on this system.", file=sys.stderr)
            return 1
        if ctk is None:
            print("customtkinter is required for the desktop GUI. Install it with: pip install -r requirements.txt", file=sys.stderr)
            return 1
        return ContentListApp().run()

    if args.mode == "email-copy":
        return run_cli_email_copy(args)
    return run_cli_scan(args)


if __name__ == "__main__":
    raise SystemExit(main())
