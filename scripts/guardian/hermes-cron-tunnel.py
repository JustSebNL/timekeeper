#!/usr/bin/env python3
"""Queue a Hermes cron run and wait until the gateway owns execution."""
from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

HERMES_ROOT = Path(os.environ.get("HERMES_PYTHON_ROOT", "/home/oc/.hermes/hermes-agent"))
if str(HERMES_ROOT) not in sys.path:
    sys.path.insert(0, str(HERMES_ROOT))

from agent.secret_scope import build_profile_secret_scope  # noqa: E402


def _request(method: str, url: str, key: str, body: bytes | None = None) -> tuple[int, dict]:
    request = urllib.request.Request(
        url,
        data=body,
        method=method,
        headers={
            "Authorization": f"Bearer {key}",
            "Content-Type": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            raw = response.read().decode("utf-8", errors="replace")
            return response.status, json.loads(raw) if raw else {}
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        try:
            payload = json.loads(raw)
        except json.JSONDecodeError:
            payload = {}
        return exc.code, payload


def _job(payload: dict) -> dict:
    value = payload.get("job")
    return value if isinstance(value, dict) else {}


def _gateway_run_line(job_id: str) -> str:
    cli = os.environ.get("HERMES_BIN", "/home/oc/.local/bin/hermes")
    try:
        result = subprocess.run(
            [cli, "cron", "runs", job_id, "--limit", "1"],
            capture_output=True,
            text=True,
            timeout=15,
            check=False,
        )
    except (OSError, subprocess.TimeoutExpired):
        return ""
    if result.returncode != 0:
        return ""
    return result.stdout


def main() -> int:
    job_id = os.environ.get("HERMES_WAKE_JOB_ID", "f307b233286f").strip()
    if not job_id:
        print("Hermes wake tunnel: HERMES_WAKE_JOB_ID is empty", file=sys.stderr)
        return 1
    home = Path(os.environ.get("HERMES_HOME", "/home/oc/.hermes"))
    key = build_profile_secret_scope(home).get("API_SERVER_KEY", "")
    if not key:
        print("Hermes wake tunnel: profile API_SERVER_KEY is unavailable", file=sys.stderr)
        return 1
    base_url = os.environ.get("HERMES_API_URL", "http://127.0.0.1:8642").rstrip("/")
    job_url = f"{base_url}/api/jobs/{job_id}"

    before_status, before_payload = _request("GET", job_url, key)
    before = _job(before_payload)
    if before_status < 200 or before_status >= 300 or before.get("id") != job_id:
        print(f"Hermes wake tunnel: cannot inspect cron job ({before_status})", file=sys.stderr)
        return 1
    baseline_run = before.get("last_run_at")

    trigger_status, trigger_payload = _request("POST", f"{job_url}/run", key)
    triggered = _job(trigger_payload)
    if trigger_status < 200 or trigger_status >= 300 or triggered.get("id") != job_id:
        detail = trigger_payload.get("error", "gateway rejected wake trigger")
        print(f"Hermes wake tunnel: trigger failed ({trigger_status}): {detail}", file=sys.stderr)
        return 1
    print(f"Hermes wake tunnel: gateway queued cron job {job_id}")

    try:
        timeout_seconds = max(30, int(os.environ.get("HERMES_WAKE_TIMEOUT", "1800")))
    except ValueError:
        timeout_seconds = 1800
    deadline = time.monotonic() + min(timeout_seconds, 1800)
    running_pattern = re.compile(rf"^\S+\s+running\s+job={re.escape(job_id)}\s+source=builtin\b", re.MULTILINE)
    completed_pattern = re.compile(rf"^\S+\s+completed\s+job={re.escape(job_id)}\s+source=builtin\b", re.MULTILINE)

    while time.monotonic() < deadline:
        run_listing = _gateway_run_line(job_id)
        if running_pattern.search(run_listing):
            # Running proves admission, not continuation. Keep the nudge
            # owned by this adapter until the supervised worker reaches a
            # successful terminal state and has time to record evidence.
            print("Hermes wake tunnel: gateway worker is running; awaiting verified completion")
            time.sleep(5)
            continue
        if completed_pattern.search(run_listing):
            status, payload = _request("GET", job_url, key)
            current = _job(payload)
            if status == 200 and current.get("last_run_at") != baseline_run:
                print("Hermes wake tunnel: gateway worker completed before observation")
                return 0 if current.get("last_status") in {"ok", "delivery_queued"} else 1
        time.sleep(5)

    print("Hermes wake tunnel: gateway worker did not reach successful completion", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
