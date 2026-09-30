"""Build portable packages with platform icons (Python 3.9+ and Go required)."""
import argparse
import os
from pathlib import Path
import plistlib
import shutil
import subprocess
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = ("windows-amd64", "linux-amd64", "darwin-arm64", "darwin-amd64")


def refresh_icons():
    # Pillow is only needed when changing artwork, not for normal builds.
    from PIL import Image
    with Image.open(ROOT / "image/FileIndex-logo.png") as source:
        source.convert("RGBA").resize((1024, 1024), Image.Resampling.LANCZOS).save(
            ROOT / "image/FileIndex.icns", format="ICNS")
    env = os.environ.copy()
    for key in ("GOOS", "GOARCH"):
        env.pop(key, None)
    subprocess.run([
        "go", "run", "github.com/akavel/rsrc@v0.10.2", "-ico", "image/FileIndex.ico",
        "-arch", "amd64", "-o", "icon_windows_amd64.syso",
    ], cwd=ROOT, env=env, check=True)


def build(target):
    platform, arch = target.split("-")
    package = ROOT / "dist" / ("FileIndex-" + target)
    package.mkdir(parents=True, exist_ok=True)
    if platform == "darwin":
        contents = package / "FileIndex.app/Contents"
        executable = contents / "MacOS/FileIndex"
        resources = contents / "Resources"
        resources.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / "image/FileIndex.icns", resources / "FileIndex.icns")
        with (contents / "Info.plist").open("wb") as stream:
            plistlib.dump({
                "CFBundleExecutable": "FileIndex", "CFBundleName": "File Index",
                "CFBundleDisplayName": "File Index", "CFBundleIdentifier": "org.fileindex.app",
                "CFBundlePackageType": "APPL", "CFBundleIconFile": "FileIndex.icns",
                "CFBundleInfoDictionaryVersion": "6.0",
                "CFBundleVersion": "1", "CFBundleShortVersionString": "1.0",
                "LSMinimumSystemVersion": "11.0",
            }, stream)
    else:
        executable = package / ("FileIndex.exe" if platform == "windows" else "FileIndex")
    executable.parent.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ, GOOS=platform, GOARCH=arch, CGO_ENABLED="0")
    subprocess.run(["go", "build", "-o", str(executable), "."], cwd=ROOT, env=env, check=True)
    executable.chmod(0o755)
    if platform == "linux":
        shutil.copy2(ROOT / "image/FileIndex-logo.png", package / "FileIndex.png")
        shutil.copy2(ROOT / "scripts/install-desktop.py", package / "install-desktop.py")
    shutil.copy2(ROOT / "LICENSE", package / "LICENSE")
    # Archive only release files; never include runtime data from an earlier launch.
    files = [executable, package / "LICENSE"]
    if platform == "darwin":
        files += [contents / "Info.plist", resources / "FileIndex.icns"]
    elif platform == "linux":
        files += [package / "FileIndex.png", package / "install-desktop.py"]
    if platform == "windows":
        archive = package.with_suffix(".zip")
        with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as output:
            for path in files:
                output.write(path, path.relative_to(package.parent))
    else:
        archive = package.with_suffix(".tar.gz")
        with tarfile.open(archive, "w:gz") as output:
            for path in files:
                info = output.gettarinfo(str(path), str(path.relative_to(package.parent)))
                info.mode = 0o755 if path == executable else 0o644
                with path.open("rb") as stream:
                    output.addfile(info, stream)
    print(archive, flush=True)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", choices=(*TARGETS, "all"), default="all")
    parser.add_argument("--refresh-icons", action="store_true")
    args = parser.parse_args()
    if args.refresh_icons:
        refresh_icons()
    for target in TARGETS if args.target == "all" else (args.target,):
        build(target)
