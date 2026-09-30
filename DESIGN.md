# SLOServe design

Status: v1 implemented locally; real-GPU measurement and cloud rollout are deliberately gated.

## 1. Problem

LLM inference overload is different from an ordinary API traffic spike:

- Work is heterogeneous. Prompt plus output tokens approximate compute and KV-cache pressure better than requests/second alone.
- A request can occupy a scarce GPU sequence for seconds or minutes.
- Queueing raises TTFT before CPU utilization becomes interesting.
- Once a token has reached the client, retrying invisibly can duplicate text and side effects.
- “HTTP 200” is not success if TTFT or TPOT missed the product SLO.

SLOServe protects latency-sensitive traffic by making admission a first-class policy rather than letting every request wait until it times out.

## 2. Scope and non-goals

### v1

- One stateless Go gateway.
- OpenAI-compatible chat completion API.
- Fake engine for CI and failure injection.
- Adapter for an OpenAI-compatible local or remote engine (`llama.cpp`, `vLLM`).
- Interactive and batch request classes.
- Token-aware bounded queue, priority, explicit shedding, cancellation, metrics, and benchmark artifacts.
- Single-GPU deployment path; Kubernetes autoscaling manifests are a controlled experiment, not a requirement to run locally.

### Non-goals

- Reimplement PagedAttention, continuous batching, CUDA kernels, or the model runtime.
- Train or finetune models.
- Add Kafka, Redis, Postgres, or a vector database without a workload that needs them.
- Claim production GPU performance from a simulator.

## 3. Architecture

```text
Browser / OpenAI SDK / benchmark runner
                    │
                    ▼
┌──────────────── Go gateway ─────────────────┐
│ validate → classify → estimate token demand │
│ token budget / priority / reserved capacity │
│ deadline / cancellation / pre-TTFT retry    │
│ SSE proxy / metrics / tenant cache salt     │
└───────────────────┬─────────────────────────┘
                    │ OpenAI-compatible HTTP
            ┌───────┴────────┐
            ▼                ▼
      fake engine      llama.cpp / vLLM
      deterministic    local CPU/Metal / GCP GPU

Prometheus ← gateway + engine metrics
Benchmark JSON → versioned public dashboard
OTel/Grafana tracing → phase 2 after real upstream integration
```

The gateway is stateless. Durable conversation history, retrieval, and product auth belong outside this project.

## 4. Request lifecycle

1. Validate the OpenAI-style body and cap requested output at 4,096 tokens.
2. Estimate demand as `ceil(message runes / 4) + max_output_tokens`. This is intentionally conservative and replaceable with the model tokenizer later.
3. Classify from `X-SLO-Priority`; unknown/missing values become `interactive`.
4. Admit immediately when a compatible execution slot is free.
5. Batch traffic that cannot start immediately is rejected with HTTP `429` and `Retry-After: 1`. It never occupies the interactive queue.
6. Interactive traffic may wait only within both the queued-token budget and its queue deadline.
7. Hash `secret || tenant` into a cache salt; never export the tenant as a metric label.
8. Stream the upstream response. The response headers are held until the first token, allowing a bounded retry before any bytes become externally visible.
9. After the first token, any upstream failure terminates the stream with an explicit error event. It is never replayed.
10. Client cancellation cancels the upstream context and releases the admission lease.

## 5. Admission invariants

- `running ≤ max_running`.
- Batch running requests never exceed `max_running - interactive_reserve`.
- `queued_tokens ≤ max_queued_tokens`.
- Only interactive requests can enter the queue in v1.
- A lease releases exactly once.
- A canceled waiter removes its token reservation.
- Dispatch always checks interactive waiters before batch work.

The controller tests cover reserved capacity, oversized work, and canceled-waiter cleanup.

## 6. Streaming and retry semantics

| Failure point | Behavior |
|---|---|
| Before first token | Retry at most `MAX_PRE_TTFT_RETRIES`; default 1 |
| After first token | Emit terminal SSE error; never replay content |
| Client disconnect | Cancel upstream context; release capacity |
| Queue deadline | HTTP 429 with a retry hint |
| Invalid request | HTTP 400, no engine call |
| Upstream non-2xx before stream | HTTP 502 after bounded retry |

The fake engine recognizes `[fail-before]` and `[fail-after]` in a prompt so both branches stay testable without a model.

## 7. Observability

Gateway metrics use low-cardinality labels and aggregatable histograms:

- `sloserve_ttft_seconds`
- `sloserve_request_duration_seconds`
- `sloserve_queue_wait_seconds`
- `sloserve_running_requests`
- `sloserve_queued_requests`
- `sloserve_queued_tokens`
- `sloserve_requests_total{outcome}`
- `sloserve_goodput_requests_total`
- `sloserve_retries_total`
- `sloserve_output_tokens_total`

Prompts, tenant IDs, request IDs, and generated text are forbidden as metric labels. Request IDs remain in logs/traces only.

## 8. Autoscaling signal

CPU is not the primary scaling signal. The first GKE experiment uses waiting requests and queued tokens; a vLLM deployment can additionally use `vllm:num_requests_waiting` and KV-cache utilization. Scale-down cooldown must be longer than model warm-up, and startup/readiness must remain separate from liveness.

Cloud Run GPU scales on request concurrency rather than GPU utilization. Therefore Cloud Run concurrency must be benchmarked against vLLM `max_num_seqs`; it is not copied from a generic web-service default.

## 9. Acceptance gates

### Implemented and automated

- Streaming output is OpenAI-compatible SSE and ends with `[DONE]`.
- Pre-token failure retries once.
- Post-token failure emits an error and never claims completion.
- Interactive capacity remains available while batch capacity is saturated.
- Canceled waiters leave no queued-token leak.
- Metrics expose cumulative histogram buckets and queue gauges.

### Required before a real GPU result is published

- Pin model revision, serving image digest, machine type, region, gateway commit, and all engine flags.
- Record cold and warm runs separately.
- Warm up, measure for at least five minutes, repeat three times.
- Capture p50/p95/p99 TTFT, TPOT, E2E, input/output throughput, goodput, queue wait, error/shed/cancel counts, KV usage, GPU utilization/memory, and cost/1M output tokens.
- Keep raw JSON and configuration beside every chart.
- State negative or neutral results; do not manufacture a speedup.

## 10. Decisions deferred on purpose

- Exact tokenizer in the gateway: use model-aware tokenization only if estimate error materially harms admission.
- OTel/Tempo: add with the real upstream so traces cover gateway and engine phases rather than a fake-only demo.
- KEDA vs managed custom metrics: choose after observing the target GKE environment.
- TensorRT-LLM/Triton comparison: only on identical model, hardware, precision, and workload.
- Custom scheduler or paged-KV allocator: a separate runtime-learning project, not hidden inside this control plane.
