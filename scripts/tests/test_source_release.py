"""Exercise publication operations in isolated temporary Git repositories."""
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location(
    "source_release", Path(__file__).resolve().parents[1] / "source-release.py"
)
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class PublicationTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.original = release.ROOT, release.MIRROR, release.STATE
        self.addCleanup(self.restore)
        release.ROOT = self.root = Path(self.temporary.name) / "source"
        self.root.mkdir()
        release.MIRROR = self.root / "bakagit"
        release.STATE = self.root / "runtime/source-publication"
        release.STATE.mkdir(parents=True)
        self.write("VERSION", "1.2.3\n")
        self.write("CHANGELOG.md", "# 更新记录\n\n## 1.2.3 - 2026-10-05\n\n- 初始版本。\n")
        self.write("internal/version/version.go", 'package version\nconst Version = "1.2.3"\n')
        self.write("web/package.json", json.dumps({"version": "1.2.3"}))
        self.write("web/package-lock.json", json.dumps({"version": "1.2.3", "packages": {"": {"version": "1.2.3"}}}))
        self.write("source-publication.json", json.dumps({
            "source_directories": ["internal", "web"],
            "source_files": ["VERSION", "CHANGELOG.md", "source-publication.json", ".env.example"],
        }))
        self.write(".env.example", "PROXY_API_KEY=\n")
        release.git(self.root, "init", "-b", "main")

    def restore(self):
        release.ROOT, release.MIRROR, release.STATE = self.original

    def write(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)
        return path

    def sync(self):
        with contextlib.redirect_stdout(io.StringIO()), release.locked():
            release.sync()

    def test_empty_git_sync_has_no_commits_remotes_or_private_data(self):
        self.write(".env", "PROXY_API_KEY=private-test\n")
        self.write("runtime/user.db", "private data")
        self.write("web/node_modules/example/index.js", "installed dependency")
        self.sync()
        self.assertFalse((release.MIRROR / ".env").exists())
        self.assertFalse((release.MIRROR / "runtime").exists())
        self.assertFalse((release.MIRROR / "web/node_modules").exists())
        self.assertEqual(release.git(release.MIRROR, "remote"), "")
        with self.assertRaises(subprocess.CalledProcessError):
            release.git(release.MIRROR, "rev-parse", "--verify", "HEAD")

    def test_existing_history_is_preserved_without_auto_commit(self):
        release.git(self.root, "add", "VERSION")
        release.git(self.root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.test",
                    "commit", "-m", "fixture")
        original = release.git(self.root, "rev-parse", "HEAD")
        self.sync()
        self.assertEqual(release.git(release.MIRROR, "rev-parse", "HEAD"), original)
        self.assertEqual(release.git(release.MIRROR, "remote", "get-url", "--push", "development"), "DISABLED")

    def test_command_directory_and_embedded_data_are_not_build_outputs(self):
        policy = json.loads((self.root / "source-publication.json").read_text())
        policy["source_directories"].append("cmd")
        policy["source_assets"] = ["internal/waa/timezones.json.gz"]
        self.write("source-publication.json", json.dumps(policy))
        self.write("cmd/aistudio2api/main.go", "package main")
        self.write("internal/waa/timezones.json.gz", "fixture source data")
        self.write("aistudio2api", "compiled binary")
        self.sync()
        self.assertTrue((release.MIRROR / "cmd/aistudio2api/main.go").is_file())
        self.assertTrue((release.MIRROR / "internal/waa/timezones.json.gz").is_file())
        self.assertFalse((release.MIRROR / "aistudio2api").exists())

    def test_sync_updates_modes_and_removes_only_managed_source(self):
        script = self.write("web/tool.js", "first")
        script.chmod(0o755)
        self.sync()
        mirrored = release.MIRROR / "web/tool.js"
        self.assertTrue(mirrored.stat().st_mode & 0o100)
        script.write_text("second")
        script.chmod(0o644)
        self.sync()
        self.assertEqual(mirrored.read_text(), "second")
        self.assertFalse(mirrored.stat().st_mode & 0o100)
        script.unlink()
        self.sync()
        self.assertFalse(mirrored.exists())

    def test_unknown_mirror_file_is_not_deleted(self):
        self.sync()
        extra = release.MIRROR / "personal.txt"
        extra.write_text("user data")
        with self.assertRaisesRegex(ValueError, "非受管文件"):
            self.sync()
        self.assertEqual(extra.read_text(), "user data")

    def test_forbidden_mirror_file_and_hash_drift_fail(self):
        self.sync()
        (release.MIRROR / "VERSION").write_text("changed")
        with self.assertRaisesRegex(ValueError, "不同步"):
            release.check()
        self.sync()
        (release.MIRROR / ".env").write_text("private")
        with self.assertRaisesRegex(ValueError, "禁止文件"):
            self.sync()

    def test_unclassified_source_and_private_key_are_rejected(self):
        extra = self.write("unknown.txt", "review me")
        with self.assertRaisesRegex(ValueError, "未分类"):
            self.sync()
        extra.unlink()
        self.write("web/private.txt", "-----BEGIN " + "PRIVATE KEY-----\nsecret")
        with self.assertRaisesRegex(ValueError, "疑似密钥"):
            self.sync()
        self.assertFalse(release.MIRROR.exists())

    def test_bump_updates_every_declaration_and_mirror(self):
        self.sync()
        for kind, expected in [("patch", "1.2.4"), ("minor", "1.3.0"), ("major", "2.0.0")]:
            with contextlib.redirect_stdout(io.StringIO()), release.locked():
                release.bump(kind, ["测试版本递增"])
            self.assertEqual(release.version_check(self.root), expected)
            self.assertEqual(release.version_check(release.MIRROR), expected)


if __name__ == "__main__":
    unittest.main()
