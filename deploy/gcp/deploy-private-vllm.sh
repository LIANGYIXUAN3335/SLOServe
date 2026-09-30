#!/usr/bin/env bash
set -euo pipefail

: "${GCP_PROJECT_ID:?Set GCP_PROJECT_ID to the billing project.}"
SLOSERVE_GCP_REGION="${GCP_REGION:-us-central1}"
SLOSERVE_GCP_SERVICE="${GCP_SERVICE_NAME:-sloserve-vllm}"
SLOSERVE_GCP_REPOSITORY="${GCP_ARTIFACT_REPOSITORY:-sloserve}"
SLOSERVE_GCP_IMAGE="${SLOSERVE_GCP_REGION}-docker.pkg.dev/${GCP_PROJECT_ID}/${SLOSERVE_GCP_REPOSITORY}/vllm:v0.10.2-qwen2.5-1.5b"

gcloud services enable \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  run.googleapis.com \
  --project "${GCP_PROJECT_ID}"

gcloud artifacts repositories describe "${SLOSERVE_GCP_REPOSITORY}" \
  --project "${GCP_PROJECT_ID}" \
  --location "${SLOSERVE_GCP_REGION}" >/dev/null 2>&1 || \
gcloud artifacts repositories create "${SLOSERVE_GCP_REPOSITORY}" \
  --project "${GCP_PROJECT_ID}" \
  --location "${SLOSERVE_GCP_REGION}" \
  --repository-format docker

gcloud builds submit deploy/gcp \
  --project "${GCP_PROJECT_ID}" \
  --tag "${SLOSERVE_GCP_IMAGE}"

gcloud run deploy "${SLOSERVE_GCP_SERVICE}" \
  --project "${GCP_PROJECT_ID}" \
  --region "${SLOSERVE_GCP_REGION}" \
  --image "${SLOSERVE_GCP_IMAGE}" \
  --gpu 1 \
  --gpu-type nvidia-l4 \
  --no-gpu-zonal-redundancy \
  --cpu 4 \
  --memory 16Gi \
  --no-cpu-throttling \
  --concurrency 4 \
  --min 0 \
  --max 1 \
  --timeout 900 \
  --ingress all \
  --no-allow-unauthenticated
