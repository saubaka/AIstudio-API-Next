#!/usr/bin/python3
"""Manage this checkout's local macOS service without keeping a terminal open."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import signal
import subprocess
import time
import urllib.request

ROOT = Path(__file__).resolve().parent
BINARY = ROOT / "aistudio2api"
RUNTIME = ROOT / "runtime"
PID_FILE = RUNTIME / "local-service.pid"
URL = "http://127.0.0.1:2048"


def owned_pid():
    try:
        pid = int(PID_FILE.read_text().strip())
        if pid <= 1:
            return None
        result = subprocess.run(
            ["/bin/ps", "-p", str(pid), "-o", "comm="],
            capture_output=True, text=True, check=False,
        )
        if result.returncode == 0 and result.stdout.strip() == str(BINARY):
            return pid
    except (OSError, ValueError):
        pass
    return None


def request(path):
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    with opener.open(URL + path, timeout=2) as response:
        return json.load(response)


def environment():
    env = os.environ.copy()
    for line in (ROOT / ".env").read_text().splitlines():
        if line.startswith("PROXY="):
            proxy = line.partition("=")[2].strip()
            if proxy:
                for key in ("HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY",
                            "http_proxy", "https_proxy", "all_proxy"):
                    env[key] = proxy
    for key in ("NO_PROXY", "no_proxy"):
        env[key] = "127.0.0.1,localhost,::1," + env.get(key, "")
    return env


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("action", choices=("start", "stop", "status", "login"))
    parser.add_argument("--no-open", action="store_true")
    args = parser.parse_args()
    RUNTIME.mkdir(exist_ok=True)
    with (RUNTIME / "local-service.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        pid = owned_pid()
        if args.action == "stop":
            if pid:
                os.kill(pid, signal.SIGTERM)
                deadline = time.monotonic() + 20
                while owned_pid() and time.monotonic() < deadline:
                    time.sleep(0.25)
                if owned_pid():
                    raise SystemExit("服务仍在退出，请检查 runtime/logs/service.log。")
            PID_FILE.unlink(missing_ok=True)
            print("本地管理服务已停止。")
            return
        if args.action == "status":
            print(json.dumps({"pid": pid, "url": URL,
                              "status": request("/api/status") if pid else "STOPPED"},
                             ensure_ascii=False, indent=2))
            return
        if not pid:
            if not BINARY.is_file():
                raise SystemExit("缺少 aistudio2api，请先构建项目。")
            logs = RUNTIME / "logs"
            logs.mkdir(exist_ok=True)
            with (logs / "service.log").open("ab", buffering=0) as log:
                process = subprocess.Popen(
                    [str(BINARY), "-open-ui=false"], cwd=ROOT, env=environment(),
                    stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                    start_new_session=True,
                )
            PID_FILE.write_text(str(process.pid) + "\n")
            pid = process.pid
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            if owned_pid() != pid:
                raise SystemExit("服务启动失败，请检查 runtime/logs/service.log。")
            try:
                if request("/health").get("status") == "ok":
                    print(f"本地管理服务已就绪：{URL}（PID {pid}）")
                    if not args.no_open:
                        subprocess.run(["/usr/bin/open", URL + ("/#google-login" if args.action == "login" else "")], check=False)
                    return
            except (OSError, ValueError):
                pass
            time.sleep(0.5)
        raise SystemExit("服务仍在准备，请检查 runtime/logs/service.log 后重试。")


if __name__ == "__main__":
    main()
