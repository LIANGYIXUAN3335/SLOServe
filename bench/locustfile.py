"""Optional Locust workload for longer interactive/batch traffic experiments."""

from locust import HttpUser, between, task


class InferenceUser(HttpUser):
    wait_time = between(0.05, 0.25)

    @task(4)
    def interactive(self) -> None:
        self._chat("interactive", 10)

    @task(1)
    def batch(self) -> None:
        self._chat("batch", 14)

    def _chat(self, priority: str, max_tokens: int) -> None:
        with self.client.post(
            "/v1/chat/completions",
            json={
                "model": "fake-engine/v1",
                "messages": [{"role": "user", "content": "Explain token-aware admission."}],
                "max_tokens": max_tokens,
                "stream": False,
            },
            headers={"X-SLO-Priority": priority, "X-Tenant-ID": "locust-demo"},
            name=f"chat/{priority}",
            catch_response=True,
        ) as response:
            if response.status_code == 429 and priority == "batch":
                response.success()
