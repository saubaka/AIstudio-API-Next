#!/usr/bin/env python3
"""Single-source publication mirror. Never commits, pushes, tags or deploys."""
import argparse
from contextlib import contextmanager
from datetime import date
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
MIRROR = ROOT / "bakagit"
STATE = ROOT / "runtime" / "source-publication"
VERSION_RE = re.compile(r"(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)")
IGNORED_DIRS = {
    ".git", "bakagit", "runtime", "auth", "node_modules", "__pycache__",
    ".venv", ".venv-macos", "venv", ".idea", ".vscode", ".pytest_cache",
    "dist", "build", "coverage", "logs", "backups", "backup", "uploads",
    "user-data", "session-data", "browser-profile", ".cache",
}
IGNORED_SUFFIXES = {
    ".db", ".sqlite", ".sqlite3", ".log", ".bak", ".backup", ".pyc", ".pyo",
    ".exe", ".dll", ".so", ".dylib", ".key", ".pem", ".p12", ".pfx",
    ".zip", ".tar", ".gz", ".dmg", ".pid", ".tmp", ".swp",
}
SECRET_PATTERNS = [
    re.compile(rb"-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----"),
    re.compile(rb"\bAIza[0-9A-Za-z_-]{35}\b"),
    re.compile(rb"\b(?:gh[pousr]_[A-Za-z0-9]{36,}|github_pat_[A-Za-z0-9_]{50,})\b"),
    re.compile(rb"\bsk-(?:proj-|ant-)?[A-Za-z0-9_-]{40,}\b"),
]


def git(directory, *args):
    return subprocess.run(
        ["git", "-C", str(directory), *args], check=True,
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    ).stdout.strip()


@contextmanager
def locked():
    STATE.mkdir(parents=True, exist_ok=True)
    with (STATE / "sync.lock").open("a+b") as handle:
        if os.name == "nt":
            import msvcrt
            handle.seek(0)
            handle.write(b"0")
            handle.flush()
            handle.seek(0)
            msvcrt.locking(handle.fileno(), msvcrt.LK_NBLCK, 1)
        else:
            import fcntl
            fcntl.flock(handle, fcntl.LOCK_EX | fcntl.LOCK_NB)
        try:
            yield
        finally:
            if os.name == "nt":
                handle.seek(0)
                msvcrt.locking(handle.fileno(), msvcrt.LK_UNLCK, 1)


def ignored(relative):
    if relative.as_posix() == "internal/waa/timezones.json.gz":
        return False  # Required embedded source data, not a build archive.
    if any(part in IGNORED_DIRS for part in relative.parts):
        return True
    name = relative.name
    return (
        name in {".DS_Store", "Thumbs.db", "go.work", "go.work.sum"}
        or relative.as_posix() == "aistudio2api"
        or (name.startswith(".env") and name != ".env.example")
        or relative.suffix.lower() in IGNORED_SUFFIXES
        or name.endswith((".db-wal", ".db-shm", ".sqlite-wal", ".sqlite-shm", "~"))
    )


def version_check(directory):
    value = (directory / "VERSION").read_text().strip()
    if not VERSION_RE.fullmatch(value):
        raise ValueError("VERSION 必须为 X.Y.Z")
    package = json.loads((directory / "web/package.json").read_text())
    lock = json.loads((directory / "web/package-lock.json").read_text())
    go_source = (directory / "internal/version/version.go").read_text()
    if (
        package["version"] != value or lock["version"] != value
        or lock["packages"][""]["version"] != value
        or f'const Version = "{value}"' not in go_source
    ):
        raise ValueError("项目、前端、锁文件和 Go 版本不一致")
    changelog = (directory / "CHANGELOG.md").read_text()
    entries = re.findall(r"^## (\d+\.\d+\.\d+) - (\d{4}-\d{2}-\d{2})$", changelog, re.M)
    if not entries or entries[0][0] != value:
        raise ValueError("CHANGELOG.md 最新记录与项目版本不一致")
    date.fromisoformat(entries[0][1])
    return value


def record(path, relative):
    if path.is_symlink() or not path.is_file():
        raise ValueError(f"不发布符号链接或特殊文件：{path.name}")
    content = path.read_bytes()
    policy = json.loads((ROOT / "source-publication.json").read_text())
    allowed = {item["sha256"] for item in policy.get("public_protocol_identifiers", [])
               if item["file"] == relative}
    for pattern in SECRET_PATTERNS:
        for match in pattern.finditer(content):
            identifier = match.group()
            # Only a pinned Google public protocol identifier may be exempted.
            if identifier.startswith(b"AIza") and hashlib.sha256(identifier).hexdigest() in allowed:
                continue
            raise ValueError(f"疑似密钥，已阻止同步（仅显示文件名）：{path.name}")
    return {"sha256": hashlib.sha256(content).hexdigest(),
            "executable": bool(path.stat().st_mode & stat.S_IXUSR)}


def source_files():
    policy = json.loads((ROOT / "source-publication.json").read_text())
    allowed_roots = set(policy["source_directories"])
    allowed_files = set(policy["source_files"])
    for child in ROOT.iterdir():
        if ignored(Path(child.name)):
            continue
        allowed = child.name in (allowed_roots if child.is_dir() else allowed_files)
        if not allowed:
            raise ValueError(f"未分类的顶层文件或目录，请先审查并登记发布规则：{child.name}")
        if child.is_symlink():
            raise ValueError(f"不发布符号链接：{child.name}")
    files = {}
    for name in sorted(allowed_files):
        path = ROOT / name
        if not path.is_file():
            raise ValueError(f"发布所需文件缺失：{name}")
        files[name] = record(path, name)
    for name in sorted(allowed_roots):
        if not (ROOT / name).is_dir():
            raise ValueError(f"发布所需目录缺失：{name}")
        for directory, folders, names in os.walk(ROOT / name, followlinks=False):
            relative_dir = Path(directory).relative_to(ROOT)
            kept = []
            for folder in sorted(folders):
                rel = relative_dir / folder
                if ignored(rel):
                    continue
                if (ROOT / rel).is_symlink():
                    raise ValueError(f"不发布符号链接：{rel}")
                kept.append(folder)
            folders[:] = kept
            for filename in sorted(names):
                rel = relative_dir / filename
                if not ignored(rel):
                    files[rel.as_posix()] = record(ROOT / rel, rel.as_posix())
    # Also catches a source file accidentally hidden by publication exclusions.
    tracked = git(ROOT, "ls-files", "-z").split("\0")
    excluded_tracked = [name for name in tracked if name and ignored(Path(name))]
    if excluded_tracked:
        raise ValueError("Git 跟踪了禁止发布的文件：" + ", ".join(excluded_tracked))
    for asset in policy.get("source_assets", []):
        if asset not in files:
            raise ValueError(f"必要的内嵌源码资源缺失：{asset}")
    return dict(sorted(files.items()))


def mirror_files():
    files = {}
    if not MIRROR.is_dir() or MIRROR.is_symlink():
        raise ValueError("没有有效的 bakagit，请先执行 sync")
    for directory, folders, names in os.walk(MIRROR, followlinks=False):
        rel_dir = Path(directory).relative_to(MIRROR)
        for folder in folders:
            if folder == ".git" and rel_dir == Path("."):
                continue
            if (Path(directory) / folder).is_symlink():
                raise ValueError(f"发布目录含符号链接：{rel_dir / folder}")
            if ignored(rel_dir / folder):
                raise ValueError(f"发布目录含禁止目录：{rel_dir / folder}")
        if rel_dir == Path("."):
            folders[:] = [name for name in folders if name != ".git"]
        for name in sorted(names):
            rel = rel_dir / name
            if rel.as_posix() == ".git":
                raise ValueError("bakagit 必须持有独立的现有历史，不能链接其他工作树")
            if ignored(rel):
                raise ValueError(f"发布目录含禁止文件：{rel}")
            files[rel.as_posix()] = record(MIRROR / rel, rel.as_posix())
    return dict(sorted(files.items()))


def source_head():
    try:
        return git(ROOT, "rev-parse", "--verify", "HEAD")
    except subprocess.CalledProcessError:
        return None  # Newly initialized history intentionally has no commits.


def ensure_history():
    if not MIRROR.exists():
        if source_head():
            # Retain existing reachable history, without sharing object files.
            subprocess.run(
                ["git", "clone", "--no-local", "--no-checkout", str(ROOT), str(MIRROR)],
                check=True, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE,
            )
            git(MIRROR, "remote", "rename", "origin", "development")
            git(MIRROR, "remote", "set-url", "--push", "development", "DISABLED")
        else:
            # The user explicitly reset this project's history; do not auto-commit.
            MIRROR.mkdir()
            git(MIRROR, "init", "-b", "main")
    if not (MIRROR / ".git").is_dir():
        raise ValueError("发布目录缺少独立 Git 历史，拒绝重新初始化或覆盖")
    # Does not reset HEAD: later authorized mirror commits remain intact.
    ensure_existing_history()


def check(source_only=False):
    version = version_check(ROOT)
    expected = source_files()
    if not source_only:
        ensure_existing_history()
        actual = mirror_files()
        if expected != actual:
            differences = sorted(name for name in set(expected) | set(actual)
                                 if expected.get(name) != actual.get(name))
            raise ValueError("镜像不同步：" + ", ".join(differences[:12]))
        version_check(MIRROR)
    print(f"校验通过：v{version}，{len(expected)} 个源码文件，"
          + ("版本及源码安全扫描通过" if source_only else "文件 SHA-256、可执行权限一致，Git 元数据检查通过"))
    return expected


def ensure_existing_history():
    if not (MIRROR / ".git").is_dir():
        raise ValueError("发布目录不存在或没有 Git 历史，请先执行 sync")
    head = source_head()
    if head:
        git(MIRROR, "merge-base", "--is-ancestor", head, "HEAD")


def sync():
    version_check(ROOT)
    expected = source_files()
    ensure_history()
    actual = mirror_files()
    manifest = STATE / "manifest.json"
    previous = json.loads(manifest.read_text()).get("files", {}) if manifest.exists() else {}
    extras = set(actual) - set(expected)
    if extras - set(previous):
        raise ValueError("镜像含非受管文件，拒绝删除：" + ", ".join(sorted(extras - set(previous))))
    # Delete only source paths recorded by a previous successful sync.
    for name in sorted(extras):
        (MIRROR / name).unlink()
    for name, metadata in expected.items():
        if actual.get(name) == metadata:
            continue
        target = MIRROR / name
        target.parent.mkdir(parents=True, exist_ok=True)
        temporary = target.with_name(target.name + ".sync-tmp")
        shutil.copyfile(ROOT / name, temporary)
        temporary.chmod(0o755 if metadata["executable"] else 0o644)
        os.replace(temporary, target)
    check()
    manifest.write_text(json.dumps({"version": version_check(ROOT), "files": expected},
                                   ensure_ascii=False, indent=2) + "\n")
    manifest.chmod(0o600)


def bump(kind, notes):
    old = version_check(ROOT)
    source_files()  # Reject unsafe source before modifying versions.
    if not notes or any("\n" in note or not re.search(r"[\u4e00-\u9fff]", note) for note in notes):
        raise ValueError("每条 --note 必须为单行简体中文更改说明")
    values = list(map(int, old.split(".")))
    index = {"major": 0, "minor": 1, "patch": 2}[kind]
    values[index] += 1
    for lower in range(index + 1, 3):
        values[lower] = 0
    value = ".".join(map(str, values))
    (ROOT / "VERSION").write_text(value + "\n")
    for name in ("web/package.json", "web/package-lock.json"):
        path = ROOT / name
        data = json.loads(path.read_text())
        data["version"] = value
        if "packages" in data:
            data["packages"][""]["version"] = value
        path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
    path = ROOT / "internal/version/version.go"
    path.write_text(path.read_text().replace(f'const Version = "{old}"', f'const Version = "{value}"'))
    path = ROOT / "CHANGELOG.md"
    title, rest = path.read_text().split("\n", 1)
    entry = f"\n## {value} - {date.today().isoformat()}\n\n"
    entry += "".join(f"- {note}\n" for note in notes)
    path.write_text(title + "\n" + entry + rest)
    sync()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    actions = parser.add_subparsers(dest="action", required=True)
    actions.add_parser("sync")
    verifier = actions.add_parser("check")
    verifier.add_argument("--source-only", action="store_true", help="仅校验源码及版本，用于 CI")
    bumper = actions.add_parser("bump")
    bumper.add_argument("kind", choices=("patch", "minor", "major"))
    bumper.add_argument("--note", action="append", required=True)
    args = parser.parse_args()
    if ROOT.name == "bakagit" and (ROOT.parent / "source-publication.json").is_file():
        raise ValueError("请从开发目录运行工具，不直接维护 bakagit")
    with locked():
        if args.action == "sync":
            sync()
        elif args.action == "check":
            check(args.source_only)
        else:
            bump(args.kind, args.note)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        # Never print captured Git stderr or file content, which may contain credentials.
        message = "Git 操作失败，请检查本地历史及工作副本状态" if isinstance(error, subprocess.CalledProcessError) else str(error)
        print("发布准备失败：" + message, file=sys.stderr)
        sys.exit(1)
