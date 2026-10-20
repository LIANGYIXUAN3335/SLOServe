#!/usr/bin/env python3
"""Evaluate a benchmark report against explicit SLO and regression gates.

Policies keep the acceptance criteria beside the workload instead of burying
them in a shell assertion. A candidate run can also be compared with a
committed baseline so CI catches statistically large latency or throughput
regressions while still enforcing absolute product SLOs.
"""

from __future__ import annotations

import argparse
import json
import operator
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable


@dataclass(frozen=True)
class Result:
    passed: bool
    message: str


OPERATORS: dict[str, tuple[str, Callable[[float, float], bool]]] = {
    "eq": ("==", operator.eq),
    "gte": (">=", operator.ge),
    "lte": ("<=", operator.le),
}


def metric(document: dict[str, Any], path: str) -> float:
    value: Any = document
    for part in path.split("."):
        if not isinstance(value, dict) or part not in value:
            raise KeyError(f"metric {path!r} is missing at {part!r}")
        value = value[part]
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise TypeError(f"metric {path!r} must be numeric, got {value!r}")
    return float(value)


def evaluate(
    report: dict[str, Any],
    policy: dict[str, Any],
    baseline: dict[str, Any] | None = None,
) -> list[Result]:
    results: list[Result] = []
    for check in policy.get("checks", []):
        path = check["metric"]
        op_name = check["op"]
        expected = float(check["value"])
        if op_name not in OPERATORS:
            raise ValueError(f"unsupported operator {op_name!r}")
        symbol, compare = OPERATORS[op_name]
        actual = metric(report, path)
        passed = compare(actual, expected)
        results.append(Result(passed, f"{path}: {actual:g} {symbol} {expected:g}"))

    comparisons = policy.get("comparisons", [])
    if comparisons and baseline is None:
        raise ValueError("policy contains baseline comparisons but no baseline was supplied")
    for check in comparisons:
        path = check["metric"]
        direction = check["better"]
        allowance = float(check["max_regression_percent"])
        actual = metric(report, path)
        baseline_value = metric(baseline or {}, path)
        if baseline_value == 0:
            raise ValueError(f"baseline metric {path!r} is zero; relative comparison is undefined")
        if direction == "lower":
            regression = 100 * (actual - baseline_value) / abs(baseline_value)
        elif direction == "higher":
            regression = 100 * (baseline_value - actual) / abs(baseline_value)
        else:
            raise ValueError(f"unsupported comparison direction {direction!r}")
        passed = regression <= allowance
        results.append(
            Result(
                passed,
                (
                    f"{path}: candidate={actual:g}, baseline={baseline_value:g}, "
                    f"regression={regression:.2f}% <= {allowance:g}%"
                ),
            )
        )
    return results


def load_json(path: Path) -> dict[str, Any]:
    value = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise TypeError(f"{path} must contain a JSON object")
    return value


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Gate an SLOServe benchmark report")
    parser.add_argument("report", type=Path, help="candidate benchmark JSON")
    parser.add_argument("--policy", type=Path, required=True, help="SLO gate policy JSON")
    parser.add_argument("--baseline", type=Path, help="committed reference JSON")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    policy = load_json(args.policy)
    results = evaluate(
        load_json(args.report),
        policy,
        load_json(args.baseline) if args.baseline else None,
    )
    name = policy.get("name", args.policy.stem)
    print(f"SLO gate: {name}")
    for result in results:
        print(f"{'PASS' if result.passed else 'FAIL'}  {result.message}")
    failures = [result for result in results if not result.passed]
    if failures:
        raise SystemExit(f"{len(failures)} of {len(results)} gates failed")
    print(f"PASS  all {len(results)} gates")


if __name__ == "__main__":
    main()
