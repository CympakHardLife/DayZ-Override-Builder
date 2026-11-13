#!/usr/bin/env python3
"""Restore Go package directories from this repository's flat source archive.

Original source files are preserved. Output is a new build/source directory.
Python 3.8+; no third-party packages required.
"""
import re
import shutil
from pathlib import Path


def main():
    root = Path(__file__).resolve().parent
    source = root / "source"
    output = root / "build" / "source"
    if output.exists():
        raise SystemExit("build/source already exists. Rename it before preparing a fresh copy.")
    planned = [(source / "go.mod", output / "go.mod")]
    for path in sorted(source.glob("*.go")):
        match = re.search(r"^package\s+(\w+)", path.read_text(encoding="utf-8-sig"), re.M)
        if not match:
            raise SystemExit("No Go package declaration: " + path.name)
        package = match.group(1)
        folder = output if package == "main" else output / package
        planned.append((path, folder / path.name))
    for name in ("app.js", "i18n.js", "index.html", "style.css"):
        planned.append((source / name, output / "web" / "static" / name))
    for original, _ in planned:
        if not original.is_file():
            raise SystemExit("Missing source file: " + str(original))
    for original, destination in planned:
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(original, destination)
    print("Prepared:", output)
    print("Next: cd build/source")
    print("      go test ./...")
    print("      go build -o DayZOverrideBuilder.exe .")


if __name__ == "__main__":
    main()
