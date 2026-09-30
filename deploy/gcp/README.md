# Private GCP GPU experiment

The public portfolio page should remain a frozen-data dashboard. The GPU engine is an experiment target: private, scale-to-zero, and capped at one L4 instance.

## Before spending money

1. Choose a billing project and confirm Cloud Run GPU quota in `us-central1` (or another supported region).
2. Create a budget alert. An alert is not a hard spending cap.
3. Review `deploy-private-vllm.sh`; it builds an Artifact Registry image and creates a billable Cloud Run service.
4. Keep the service private. Do not add `--allow-unauthenticated`.

## Deploy

```bash
export GCP_PROJECT_ID=your-project-id
export GCP_REGION=us-central1
./deploy/gcp/deploy-private-vllm.sh
```

The deployment sets `min=0`, `max=1`, four-request concurrency, no CPU throttling, and one NVIDIA L4. Cloud Run does not scale from GPU utilization, so concurrency must be established by benchmark rather than copied blindly.

## Run the local gateway against it

Use an authenticated local proxy; the private endpoint is never exposed directly:

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

`deploy/k8s/base` contains the gateway and one vLLM replica. The `gke` folder adds Google Managed Prometheus and a KEDA example that scales on pending inference requests. The example is deliberately not part of the base kustomization: install its CRDs and verify the metric name before applying it.
