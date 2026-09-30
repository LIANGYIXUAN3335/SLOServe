#!/usr/bin/env python3
"""Dependency-free SLOServe benchmark runner.

Measures TTFT from request start to the first SSE chunk and records end-to-end
latency, completed output chunks, HTTP outcome, and SLO goodput. Raw samples and
the exact run configuration are written together so a chart is never detached
from its workload.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import math
import subprocess
import statistics
import time
import urllib.error
import urllib.request
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


@dataclass(frozen=True)
class Sample:
    request_id: int
    priority: str
    status: int
    ttft_ms: float | None
    mean_inter_chunk_ms: float | None
    e2e_ms: float
    output_chunks: int
    completed: bool
    error: str | None


def percentile(values: list[float], p: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    rank = (len(ordered) - 1) * p
    lower = math.floor(rank)
    upper = math.ceil(rank)
    if lower == upper:
        return ordered[lower]
    return ordered[lower] + (ordered[upper] - ordered[lower]) * (rank - lower)


def run_one(
    request_id: int,
    endpoint: str,
    prompt: str,
    max_tokens: int,
    priority: str,
    timeout: float,
) -> Sample:
    body = json.dumps(
        {
            "model": "fake-engine/v1",
            "messages": [{"role": "user", "content": prompt}],
            "stream": True,
            "max_tokens": max_tokens,
        }
    ).encode()
    request = urllib.request.Request(
        endpoint.rstrip("/") + "/v1/chat/completions",
        data=body,
        headers={
            "Content-Type": "application/json",
            "X-SLO-Priority": priority,
            "X-Tenant-ID": f"benchmark-tenant-{request_id % 4}",
        },
        method="POST",
    )
    started = time.perf_counter()
    first_chunk_at: float | None = None
    last_chunk_at: float | None = None
    chunks = 0
    completed = False
    status = 0
    error: str | None = None
    current_event = "message"
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            status = response.status
            for raw_line in response:
                line = raw_line.decode("utf-8", "replace").strip()
                if not line:
                    current_event = "message"
                    continue
                if line.startswith("event:"):
                    current_event = line.removeprefix("event:").strip()
                    continue
                if not line.startswith("data:"):
                    continue
                payload = line.removeprefix("data:").strip()
                if current_event == "error":
                    try:
                        message = json.loads(payload).get("error", {}).get("message")
                    except (json.JSONDecodeError, AttributeError):
                        message = None
                    error = f"upstream stream error: {message or payload}"
                    break
                if payload == "[DONE]":
                    completed = True
                    break
                try:
                    decoded = json.loads(payload)
                except json.JSONDecodeError as exc:
                    error = f"invalid SSE JSON: {exc}"
                    break
                if isinstance(decoded, dict) and decoded.get("error"):
                    message = decoded["error"].get("message") if isinstance(decoded["error"], dict) else decoded["error"]
                    error = f"upstream stream error: {message}"
                    break
                if first_chunk_at is None:
                    first_chunk_at = time.perf_counter()
                last_chunk_at = time.perf_counter()
                chunks += 1
    except urllib.error.HTTPError as exc:
        status = exc.code
        error = exc.read(512).decode("utf-8", "replace")
    except Exception as exc:  # noqa: BLE001 - benchmark must record all client failures
        error = f"{type(exc).__name__}: {exc}"
    finished = time.perf_counter()
    if status not in {429, 503} and not completed and error is None:
        error = "stream ended without [DONE]"
    return Sample(
        request_id=request_id,
        priority=priority,
        status=status,
        ttft_ms=round((first_chunk_at - started) * 1000, 3) if first_chunk_at else None,
        mean_inter_chunk_ms=round((last_chunk_at - first_chunk_at) * 1000 / (chunks - 1), 3)
        if first_chunk_at is not None and last_chunk_at is not None and chunks > 1
        else None,
        e2e_ms=round((finished - started) * 1000, 3),
        output_chunks=chunks,
        completed=completed,
        error=error,
    )


def summarize(samples: list[Sample], ttft_slo_ms: float, elapsed: float) -> dict[str, Any]:
    ttfts = [sample.ttft_ms for sample in samples if sample.ttft_ms is not None]
    intervals = [
        sample.mean_inter_chunk_ms
        for sample in samples
        if sample.mean_inter_chunk_ms is not None
    ]
    durations = [sample.e2e_ms for sample in samples]
    completed = [sample for sample in samples if sample.completed]
    good = [sample for sample in completed if sample.ttft_ms is not None and sample.ttft_ms <= ttft_slo_ms]
    shed = [sample for sample in samples if sample.status in {429, 503}]
    def priority_summary(priority: str) -> dict[str, Any]:
        selected = [sample for sample in samples if sample.priority == priority]
        selected_completed = [sample for sample in selected if sample.completed]
        selected_good = [
            sample
            for sample in selected_completed
            if sample.ttft_ms is not None and sample.ttft_ms <= ttft_slo_ms
        ]
        return {
            "requests": len(selected),
            "completed": len(selected_completed),
            "shed": sum(1 for sample in selected if sample.status in {429, 503}),
            "goodput_percent": round(100 * len(selected_good) / len(selected), 3)
            if selected
            else None,
        }

    return {
        "requests": len(samples),
        "completed": len(completed),
        "shed": len(shed),
        "errors": sum(1 for sample in samples if not sample.completed and sample not in shed),
        "request_throughput_rps": round(len(completed) / elapsed, 3) if elapsed else None,
        "output_chunk_throughput_per_second": round(
            sum(sample.output_chunks for sample in completed) / elapsed, 3
        )
        if elapsed
        else None,
        "goodput_percent": round(100 * len(good) / len(samples), 3) if samples else None,
        "by_priority": {
            "interactive": priority_summary("interactive"),
            "batch": priority_summary("batch"),
        },
        "ttft_ms": {
            "p50": round(percentile(ttfts, 0.50), 3) if ttfts else None,
            "p95": round(percentile(ttfts, 0.95), 3) if ttfts else None,
            "p99": round(percentile(ttfts, 0.99), 3) if ttfts else None,
            "mean": round(statistics.fmean(ttfts), 3) if ttfts else None,
        },
        "mean_inter_chunk_ms": {
            "p50": round(percentile(intervals, 0.50), 3) if intervals else None,
            "p95": round(percentile(intervals, 0.95), 3) if intervals else None,
            "p99": round(percentile(intervals, 0.99), 3) if intervals else None,
        },
        "e2e_ms": {
            "p50": round(percentile(durations, 0.50), 3),
            "p95": round(percentile(durations, 0.95), 3),
            "p99": round(percentile(durations, 0.99), 3),
        },
    }


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run a reproducible SLOServe SSE benchmark")
    parser.add_argument("--endpoint", default="http://127.0.0.1:8080")
    parser.add_argument("--requests", type=int, default=48)
    parser.add_argument("--concurrency", type=int, default=8)
    parser.add_argument("--max-tokens", type=int, default=10)
    parser.add_argument("--batch-ratio", type=float, default=0.35)
    parser.add_argument("--ttft-slo-ms", type=float, default=300)
    parser.add_argument("--timeout", type=float, default=15)
    parser.add_argument(
        "--prompt",
        default="Summarize why token-aware admission protects interactive inference SLOs.",
    )
    parser.add_argument("--output", type=Path, default=Path("bench/results/latest.json"))
    return parser.parse_args()


def fetch_server_profile(endpoint: str) -> dict[str, Any]:
    request = urllib.request.Request(endpoint.rstrip("/") + "/debug/state")
    try:
        with urllib.request.urlopen(request, timeout=2) as response:
            return json.load(response)
    except Exception as exc:  # noqa: BLE001 - retain the measurement with a warning
        return {"unavailable": f"{type(exc).__name__}: {exc}"}


def git_revision() -> dict[str, Any]:
    root = Path(__file__).resolve().parent.parent
    try:
        revision = subprocess.run(
            ["git", "rev-parse", "HEAD"],
            cwd=root,
            check=True,
            capture_output=True,
            text=True,
        ).stdout.strip()
        dirty = bool(
            subprocess.run(
                ["git", "status", "--porcelain"],
                cwd=root,
                check=True,
                capture_output=True,
                text=True,
            ).stdout.strip()
        )
        return {"sha": revision, "dirty": dirty}
    except (OSError, subprocess.CalledProcessError):
        return {"sha": None, "dirty": None}


def main() -> None:
    args = parse_args()
    if args.requests < 1 or args.concurrency < 1:
        raise SystemExit("requests and concurrency must be positive")
    if not 0 <= args.batch_ratio <= 1:
        raise SystemExit("batch-ratio must be between 0 and 1")

    prompt = args.prompt
    server_profile = fetch_server_profile(args.endpoint)
    started = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        futures = [
            pool.submit(
                run_one,
                request_id,
                args.endpoint,
                prompt,
                args.max_tokens,
                "batch"
                if ((request_id * 37) % 100) < round(args.batch_ratio * 100)
                else "interactive",
                args.timeout,
            )
            for request_id in range(args.requests)
        ]
        samples = [future.result() for future in concurrent.futures.as_completed(futures)]
    elapsed = time.perf_counter() - started
    samples.sort(key=lambda sample: sample.request_id)

    report = {
        "schema_version": 2,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "engine": server_profile.get("engine", {}).get("name", "unknown"),
        "gateway_revision": git_revision(),
        "server_profile": server_profile,
        "config": {
            "endpoint": args.endpoint,
            "requests": args.requests,
            "concurrency": args.concurrency,
            "max_tokens": args.max_tokens,
            "batch_ratio": args.batch_ratio,
            "ttft_slo_ms": args.ttft_slo_ms,
            "prompt_chars": len(prompt),
        },
        "elapsed_seconds": round(elapsed, 3),
        "summary": summarize(samples, args.ttft_slo_ms, elapsed),
        "samples": [asdict(sample) for sample in samples],
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report["summary"], indent=2))
    print(f"wrote {args.output}")


if __name__ == "__main__":
    main()
