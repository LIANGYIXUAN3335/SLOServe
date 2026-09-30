# IAM-protected GCP GPU experiment

The portfolio page should remain a frozen-data dashboard. The GPU engine is a short-lived experiment target: authenticated with Cloud Run IAM, scale-to-zero, and capped at one L4 instance.

## Before spending money

1. Choose a billing project and confirm Cloud Run GPU quota in `us-central1` (or another supported region). The script enables Artifact Registry, Cloud Build, and Cloud Run APIs in that project.
2. Create a budget alert. An alert is not a hard spending cap.
3. Review `deploy-private-vllm.sh`; it builds an Artifact Registry image and creates a billable Cloud Run service.
4. Keep IAM authentication required. Do not add `--allow-unauthenticated`.

## Deploy

```bash
export GCP_PROJECT_ID=your-project-id
export GCP_REGION=us-central1
./deploy/gcp/deploy-private-vllm.sh
```

The deployment sets `min=0`, `max=1`, four-request concurrency, no CPU throttling, and one NVIDIA L4. Cloud Run does not scale from GPU utilization, so concurrency must be established by benchmark rather than copied blindly.

The service uses public Cloud Run ingress but rejects callers without IAM authorization. “IAM-protected” is not the same as network-private. A VPC-only endpoint needs a separate internal-ingress design and is outside this experiment.

The image base is pinned to the vLLM v0.10.2 multi-architecture digest, and the model is pinned to Hugging Face revision `989aa7980e4cf806f80c7fef2b1adb7bc71aa306`.

## Run the local gateway against it

Use an authenticated local proxy; the vLLM API is never made anonymously callable:

```bash
gcloud run services proxy sloserve-vllm \
  --project "$GCP_PROJECT_ID" \
  --region "$GCP_REGION" \
  --port 8000
```

In another terminal:

```bash
ENGINE_MODE=openai \
UPSTREAM_URL=http://127.0.0.1:8000 \
DEFAULT_MODEL=Qwen/Qwen2.5-1.5B-Instruct \
go -C gateway run ./cmd/gateway
```

Only publish results after recording the Cloud Run revision, container digest, model revision, region, cold/warm state, exact workload, raw JSON, and full cost window.

## Delete the billable service

```bash
gcloud run services delete sloserve-vllm \
  --project "$GCP_PROJECT_ID" \
  --region "$GCP_REGION"
```

Official references: [Cloud Run GPU configuration](https://docs.cloud.google.com/run/docs/configuring/services/gpu), [GPU inference best practices](https://docs.cloud.google.com/run/docs/configuring/services/gpu-best-practices), and [Cloud Run pricing](https://cloud.google.com/run/pricing).

## GKE stretch path

See [`deploy/k8s/README.md`](../k8s/README.md). The base is a GPU-free kind smoke test. The GKE overlay is an explicit paid exception that starts one L4; its optional KEDA example can raise that to two and is never applied by default.
