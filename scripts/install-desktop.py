"""Run with python3 after placing this portable Linux package at its final location."""
import os
from pathlib import Path


def value(text):
    return str(text).replace("\\", "\\\\").replace("\n", "\\n").replace("\r", "\\r").replace("\t", "\\t")


def command(path):
    text = str(path).replace("%", "%%")
    for char in ('\\', '"', '`', '$'):
        text = text.replace(char, '\\' + char)
    return value('"' + text + '"')


if __name__ == "__main__":
    root = Path(__file__).resolve().parent
    executable = root / "FileIndex"
    executable.chmod(executable.stat().st_mode | 0o111)
    entry = ("[Desktop Entry]\nType=Application\nName=File Index\n"
             f"Exec={command(executable)}\nPath={value(root)}\n"
             f"Icon={value(root / 'FileIndex.png')}\n"
             "Terminal=false\nCategories=Utility;\n")
    local = root / "FileIndex.desktop"
    local.write_text(entry, encoding="utf-8")
    local.chmod(0o755)
    applications = Path(os.environ.get("XDG_DATA_HOME") or Path.home() / ".local/share") / "applications"
    applications.mkdir(parents=True, exist_ok=True)
    destination = applications / "fileindex.desktop"
    destination.write_text(entry, encoding="utf-8")
    print(f"Created {local}\nInstalled {destination}\nRun this script again after moving the package.")
