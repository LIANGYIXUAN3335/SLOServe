# Should this project exist? Market research, 2026-09-29

## Decision

**Yes—if it is a focused inference performance and reliability case study. No—if it is a checklist that shallowly combines every term in the original post.**

The durable hiring signal is not a particular framework name. It is evidence that the candidate can measure TTFT/TPOT/throughput, locate a GPU/KV/queue bottleneck, protect an SLO, recover from failures, and explain the cost tradeoff.

## What JobScan “鸭先知” actually says

The requested view is JobScan’s `🦆 trend` / “鸭先知,” not the Atoms learning app. The latest local snapshot was generated at 17:18 on 2026-09-29 from 8,671 retained JDs.

| Theme | JD / companies | Median time open/close signal | Last-14-day share change | Interpretation |
|---|---:|---:|---:|---|
| LLM inference/serving | 177 / 51 | 62 days, but only 5 companies met the close-time threshold | -0.3pp | scarce and specialized; not broadly heating up |
| GPU platform | 425 / 64 | 33.5 days, 6 companies | **+2.1pp** | the clearest rising AI-infra signal |
| LLM training/finetune | 319 / 91 | 23.25 days | -0.5pp | real but role-specific demand |
| AI agents | 2,005 / 420 | 4 days | -3.6pp | common, recently losing share |
| Data platform | 1,773 / 394 | 4 days | -0.8pp | large baseline market |
| Backend baseline | 1,040 / 374 | 3 days | +2.0pp | broad demand, faster churn |

Within the 177 serving JDs, the differentiating skills are coherent:

- vLLM 49.2%
- Kubernetes 45.2%
- Python 71.8%, Go 37.3%
- CUDA 40.7%, C++ 40.1%, PyTorch 37.9%
- SGLang 27.7%, TensorRT-LLM 18.1%
- NCCL and InfiniBand 14.7% each, Ray 10.7%

This supports a `serving → gateway → Kubernetes → metrics → benchmark` loop. It does not support mixing distributed training into v1.

## What the screenshot overstates

Across all stable-panel JDs in the latest 14-day window, explicit mentions were much rarer:

- vLLM: 10 JDs / 5 companies (1.09%)
- TensorRT-LLM: 4 / 1; NVIDIA Triton inference server: 3 / 1
- KV cache: 3 / 2; quantization: 5 / 3
- ONNX Runtime and “continuous batching”: 0 explicit matches
- DDP/FSDP and DeepSpeed: 1 each; Megatron: 2; NCCL: 10; Ray: 8

Mechanisms can matter without appearing verbatim in a JD, but this is a specialized market—not a universal backend requirement.

The screenshot also mixes four different jobs:

1. inference performance/runtime,
2. distributed training,
3. GPU/HPC platform,
4. AI application backend.

A single personal project should not pretend to demonstrate all four.

## Dataset caveats

The local evidence is useful for this candidate, not representative of the entire market:

- The scanner fetched 31,787 roles and retained 3,365 in the latest run (10.6%).
- It intentionally filters many ML engineer, data engineer, SRE, Staff+, non-US, and GPU-title roles.
- Inference/GPU companies were deliberately added on 2026-08-16; AI datacenter/power companies were added on 2026-09-09.
- The trend reader can include LinkedIn shadow duplicates; the upper-bound duplicate share is roughly 7%.
- The “62-day” serving close-time estimate is based on exactly five qualifying companies and a 58.3% ghost-role share, so confidence is medium.
- “Role disappeared” can mean expiry, withdrawal, crawl churn, or repost—not necessarily a successful hire.
- Skill history has only 20 days and the code itself calls for four weeks before treating arrows as stable.

Local source trail:

- `jobscan/data/trends/2026-09-29.json`
- `jobscan/app/trends.py`
- `jobscan/config.yaml` → search `trends:`
- `jobscan/templates/list.html` → search `鸭先知`

## External cross-check

The external evidence supports the direction and the narrowing:

- The [2026 Stanford AI Index / Lightcast summary](https://lightcast.io/resources/research/stanford-ai-index-2026) reports AI skills in roughly 2.5% of US job postings, up 55% year over year; cloud, scalability, and workflow skills indicate a shift from prototypes toward operation.
- LinkedIn’s [US software-engineer talent landscape](https://economicgraph.linkedin.com/content/dam/me/economicgraph/en-us/PDF/us-software-engineer-talent-landscape-2026.pdf) shows AI Engineer hiring growing rapidly while GenAI Engineer transitions remain under 1% of SWE moves: fast growth, still a small and high-bar segment.
- Current roles at [OpenAI GPT Infrastructure](https://openai.com/careers/software-engineer-gpt-infrastructure-san-francisco/), [Baseten Model Performance](https://jobs.ashbyhq.com/baseten/d29e748c-7209-460d-a024-8f77ae0a3d4d), [Fluidstack Inference Platform](https://jobs.ashbyhq.com/fluidstack/11c22225-f240-4136-a5a0-a2cfa37ca646/), and [Perplexity Inference & Training Platform](https://jobs.ashbyhq.com/perplexity/4b8347e9-615e-4176-bfb8-3be9025f9f27) repeatedly ask for measured latency, throughput, GPU/memory efficiency, cost, Kubernetes/cloud, and reliability.
- Google’s [GKE inference best practices](https://docs.cloud.google.com/kubernetes-engine/docs/best-practices/machine-learning/inference) recommend inference-aware signals such as pending queues and KV-cache pressure rather than treating CPU as the sole autoscaling input.
- vLLM exposes an [OpenAI-compatible API](https://docs.vllm.ai/en/latest/serving/online_serving/openai_compatible_server/), [serving metrics](https://docs.vllm.ai/en/latest/design/metrics/), and a [goodput-aware benchmark](https://docs.vllm.ai/en/latest/cli/bench/serve/), so the project should integrate and validate the runtime rather than reimplement it.

## Why it is especially useful for this portfolio

The local JobScan scoring notes repeatedly identify a specific credibility gap: the candidate has backend/distributed-system experience and a local RAG project, but no operated model-serving system. A measured serving project closes that gap honestly. It cannot substitute for production GPU-fleet experience, but it can replace “read the docs” with reproducible engineering evidence.

## Final project choice

Build **SLOServe**:

- Go gateway to emphasize backend concurrency and failure semantics.
- Fake engine and local `llama.cpp` path so the complete system is runnable without a GPU.
- vLLM single-GPU path for real measurement.
- TTFT, TPOT, throughput, goodput, KV/queue pressure, errors, utilization, and cost.
- One overload experiment proving priority isolation and explicit shedding.
- Public frozen-data dashboard; private scale-to-zero GPU endpoint.

Defer distributed training, Mini-vLLM, and TensorRT-LLM until a role target makes the extra depth worthwhile.
