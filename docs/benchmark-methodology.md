# Benchmark methodology

## Claim rule

No chart without its raw JSON, workload, model revision, image digest, hardware, gateway commit, and engine flags. Simulator results must say “fake engine”; they are valid for control-plane behavior only.

## Workload matrix

| Dimension     | Values                                                                     |
| ------------- | -------------------------------------------------------------------------- |
| Input tokens  | 128, 512, 2,048                                                            |
| Output tokens | 64, 256                                                                    |
| Concurrency   | 1, 4, 8, 16                                                                |
| Shapes        | short interactive, long prompt, 80% shared prefix, sudden burst            |
| Policy        | direct engine; gateway/no protection; bounded tokens + priority + shedding |
| Cache         | prefix cache off/on                                                        |
| Engine        | fake CI; local llama.cpp; vLLM on one pinned GPU                           |

## Procedure for a real model

1. Record environment and freeze all revisions.
2. Separate cold-start from warm-serving results.
3. Warm up until compile/cache/model-loading effects settle.
4. Run each cell for at least five minutes.
5. Repeat three times and retain every raw run, including failures.
6. Report medians across repetitions plus within-run p50/p95/p99.
7. Compare only identical model, precision, prompt/output distribution, and hardware.

## Metrics

- TTFT: request start to first output token.
- Mean inter-chunk interval: what the dependency-free SSE client measures today. It is not labeled TPOT because an SSE chunk is not guaranteed to contain exactly one tokenizer token.
- TPOT: derive from tokenizer-aware output counts for a pinned real engine; do not infer it from chunk count.
- ITL: individual inter-token latency distribution when the client captures timestamps.
- E2E: request start to terminal event.
- Input/output token throughput.
- Request throughput.
- Goodput: offered requests meeting all declared SLOs, not merely HTTP success.
- Queue time, waiting/running requests, queued tokens.
- KV-cache usage and prefix-cache hits.
- Explicit shed, error, cancellation, and retry counts.
- GPU utilization/memory and cost per one million output tokens.

## Reference fake-engine profile

The committed overload file is a real execution of the gateway against the deterministic engine:

```text
fake TTFT=40ms; fake TPOT=5ms
max running=4; interactive reserve=1
max queue wait=350ms; max queued tokens=4,000
48 requests; concurrency=12; 35% batch; 10 output chunks
TTFT SLO=300ms
```

It verifies policy behavior and measurements. It says nothing about a specific model or GPU.

Run `make reference` to launch that exact gateway profile. Before replacing either committed JSON artifact, the command verifies that a post-token stream failure is counted as an error and evaluates the new run against the policies in `bench/policies/`.

CI runs `SLOSERVE_UPDATE_REFERENCE=0 make reference`, which evaluates a fresh candidate without rewriting the checked-in baseline. A deliberate local `make reference` updates the artifacts only after every gate passes.

The gate has two layers:

- Absolute product checks enforce TTFT, interactive goodput, unexpected-error, and shedding semantics.
- Baseline comparisons fail on a material p95 TTFT or throughput regression, with explicit tolerances for runner noise.

This is deliberately a release gate, not a claim of statistical significance. A real-GPU comparison still requires the repeated, pinned procedure above.

## Failure experiments

- Pre-token upstream failure: one bounded retry, no bytes emitted first.
- Post-token failure: explicit terminal stream error, no replay.
- Client cancellation: upstream context cancellation and lease release.
- Queue overflow: retryable 429 within the queue deadline.
- Pod restart during load: no false successful completion.
- Readiness failure: stop new routing without killing in-flight work.

## Cost calculation

For each pinned cloud run:

```text
cost / 1M output tokens = active resource cost / output tokens × 1,000,000
```

Report idle/warm capacity separately from active-serving efficiency. Include CPU, memory, GPU, persistent storage, load-balancing, and material egress; never quote GPU price alone as total service cost.
