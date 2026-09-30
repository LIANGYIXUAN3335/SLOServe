import unittest
from unittest import mock

from bench import run


class FakeResponse:
    status = 200

    def __init__(self, lines: list[bytes]):
        self.lines = lines

    def __enter__(self):
        return self

    def __exit__(self, *_args):
        return False

    def __iter__(self):
        return iter(self.lines)


class RunOneTests(unittest.TestCase):
    @mock.patch.object(run.urllib.request, "urlopen")
    def test_terminal_sse_error_is_not_counted_as_completion(self, urlopen):
        urlopen.return_value = FakeResponse(
            [
                b'data: {"choices":[{"delta":{"content":"hello"}}]}\n',
                b'\n',
                b'event: error\n',
                b'data: {"error":{"message":"engine failed"}}\n',
                b'\n',
            ]
        )

        sample = run.run_one(1, "http://example.test", "prompt", 4, "interactive", 1)

        self.assertFalse(sample.completed)
        self.assertEqual(sample.output_chunks, 1)
        self.assertIn("engine failed", sample.error or "")

    @mock.patch.object(run.urllib.request, "urlopen")
    def test_done_marks_stream_complete(self, urlopen):
        urlopen.return_value = FakeResponse(
            [
                b'data: {"choices":[{"delta":{"content":"hello"}}]}\n',
                b'\n',
                b'data: [DONE]\n',
                b'\n',
            ]
        )

        sample = run.run_one(2, "http://example.test", "prompt", 4, "interactive", 1)

        self.assertTrue(sample.completed)
        self.assertIsNone(sample.error)
        self.assertEqual(sample.output_chunks, 1)


if __name__ == "__main__":
    unittest.main()
