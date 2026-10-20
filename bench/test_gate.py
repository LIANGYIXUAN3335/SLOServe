import unittest

from bench import gate


def report(*, errors: int = 0, goodput: float = 100, p95: float = 240, rps: float = 40):
    return {
        "summary": {
            "errors": errors,
            "request_throughput_rps": rps,
            "by_priority": {"interactive": {"goodput_percent": goodput}},
            "ttft_ms": {"p95": p95},
        }
    }


class GateTests(unittest.TestCase):
    def test_absolute_checks_report_each_result(self):
        policy = {
            "checks": [
                {"metric": "summary.errors", "op": "eq", "value": 0},
                {
                    "metric": "summary.by_priority.interactive.goodput_percent",
                    "op": "gte",
                    "value": 99,
                },
                {"metric": "summary.ttft_ms.p95", "op": "lte", "value": 300},
            ]
        }

        results = gate.evaluate(report(), policy)

        self.assertEqual([result.passed for result in results], [True, True, True])

    def test_failed_slo_is_visible(self):
        policy = {
            "checks": [
                {"metric": "summary.errors", "op": "eq", "value": 0},
                {"metric": "summary.ttft_ms.p95", "op": "lte", "value": 300},
            ]
        }

        results = gate.evaluate(report(errors=1, p95=340), policy)

        self.assertEqual([result.passed for result in results], [False, False])
        self.assertIn("340 <= 300", results[1].message)

    def test_relative_checks_respect_metric_direction(self):
        policy = {
            "comparisons": [
                {
                    "metric": "summary.ttft_ms.p95",
                    "better": "lower",
                    "max_regression_percent": 20,
                },
                {
                    "metric": "summary.request_throughput_rps",
                    "better": "higher",
                    "max_regression_percent": 20,
                },
            ]
        }

        results = gate.evaluate(
            report(p95=250, rps=31), policy, baseline=report(p95=200, rps=40)
        )

        self.assertEqual([result.passed for result in results], [False, False])

    def test_missing_metric_fails_closed(self):
        with self.assertRaisesRegex(KeyError, "goodput_percent"):
            gate.evaluate(
                {"summary": {}},
                {
                    "checks": [
                        {
                            "metric": "summary.by_priority.interactive.goodput_percent",
                            "op": "gte",
                            "value": 99,
                        }
                    ]
                },
            )


if __name__ == "__main__":
    unittest.main()
