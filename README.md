# SLOServe

**A portable, SLO-aware LLM gateway and benchmark lab.** SLOServe turns an inference latency target into concrete backend behavior: token-aware admission, bounded queues, priority isolation, cancellation propagation, and load shedding. It runs against a deterministic fake engine in CI, a local OpenAI-compatible engine such as `llama.cpp`, or `vLLM` on a GPU without changing the client contract.

This repository is intentionally narrower than a “complete AI stack” demo. It answers one engineering question well:

> When demand exceeds inference capacity, which requests should run, wait, or fail so useful tokens keep arriving within SLO?

## What is implemented

- OpenAI-compatible `POST /v1/chat/completions`, streaming and non-streaming.
- Go SSE gateway with request deadlines and cancellation propagation.
- Token-aware queue budget; a 2,000-token request and a 20-token request do not look identical.
- Reserved interactive capacity and explicit, retryable batch shedding.
- One retry only before the first token; a partially streamed response is never replayed.
- Per-tenant prefix-cache salt without putting tenant or request IDs in metric labels.
- Prometheus counters, gauges, and aggregatable histograms for TTFT, queue time, duration, goodput, retries, tokens, and shed requests.
- Dependency-free benchmark runner plus an optional Locust workload.
- An interactive portfolio dashboard with committed, downloadable reference runs.
- Docker, Prometheus/Grafana, a GPU-free Kubernetes smoke test, and opt-in GCP/GKE experiment manifests.

Not in v1: distributed training, Slurm, Kafka, a vector database, or a hand-written inference runtime. Those would blur the serving/reliability story rather than strengthen it.

## Run it locally

Requires Go 1.25+ and Node 22+.

```bash
make gateway
```

In another terminal:

```bash
curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'X-SLO-Priority: interactive' \
  -d '{"model":"fake-engine/v1","messages":[{"role":"user","content":"Why token-aware queues?"}],"stream":true,"max_tokens":8}'
```

Reproduce both committed reference workloads and the post-token failure smoke test:

```bash
make reference
```

`make bench` is the generic runner for a gateway already listening on port 8080.

Run the dashboard:

```bash
npm install
npm run dev
```

The local gateway defaults to the deterministic fake engine; it needs no model, GPU, cloud account, or secret. To proxy `llama.cpp` or `vLLM`, set `ENGINE_MODE=openai`, `UPSTREAM_URL`, and `DEFAULT_MODEL` as shown in [.env.example](.env.example).

## Reference result

The committed overload run uses a deterministic 40ms first-token delay, 5ms/token generation delay, four running slots, one slot reserved for interactive traffic, 12 concurrent clients, and a 35% batch mix.

| Outcome                             |            Result |
| ----------------------------------- | ----------------: |
| Interactive requests                | 31 / 31 completed |
| Interactive TTFT goodput (`≤300ms`) |          **100%** |
| Batch requests shed explicitly      |           16 / 17 |
| TTFT p95                            |       **244.1ms** |
| Mean inter-chunk interval p95       |  **6.56ms/chunk** |
| Unexpected errors                   |             **0** |

These numbers prove gateway behavior, not GPU or tokenizer performance. The client observes SSE chunk timing; it does not assume one chunk equals one model token. Real GPU claims will be added only from pinned model/image/hardware runs. Raw data lives in [`bench/results/`](bench/results/), including the effective server limits, fake-engine timings, and Git state.

## Read the design

- [DESIGN.md](DESIGN.md) — architecture, invariants, API behavior, failure semantics, and acceptance gates.
- [docs/research.md](docs/research.md) — JobScan “鸭先知” data, external market evidence, and why the original post needed narrowing.
- [docs/benchmark-methodology.md](docs/benchmark-methodology.md) — workload matrix and rules that keep performance claims reproducible.
- [deploy/gcp/README.md](deploy/gcp/README.md) — an IAM-protected, scale-to-zero Cloud Run GPU experiment path.
- [deploy/k8s/README.md](deploy/k8s/README.md) — a GPU-free kind smoke test and the explicit-cost GKE overlay.

## Repository map

```text
app/ + components/     portfolio benchmark dashboard
gateway/               Go gateway, engines, metrics, tests
bench/                 dependency-free runner, Locust workload, raw results
deploy/                Docker, Prometheus/Grafana, Kubernetes, GCP
docs/                  research and benchmark method
```

## Safety defaults

- The portfolio dashboard contains frozen benchmark data, never a live GPU endpoint.
- Cloud Run GPU deployment requires IAM, `min-instances=0`, and `max-instances=1`.
- GKE is a separate paid stretch path: one L4 by default; optional KEDA can raise it to two and is never applied by the base.
- Prometheus labels never contain prompts, tenants, request IDs, or model-generated text.
- Model revisions and container digests must be pinned before publishing a GPU result.
- Budget alerts are required but are not treated as a hard spending cap.

## License

MIT. See [LICENSE](LICENSE).
