# Kubernetes paths

The default manifest is deliberately GPU-free. The GKE overlay is a separate, paid experiment.

## Local kind smoke test

Build and load the gateway image before applying the base:

```bash
docker build -t sloserve-gateway:local gateway
kind load docker-image sloserve-gateway:local
kubectl apply -k deploy/k8s/base
kubectl -n sloserve port-forward service/sloserve-gateway 8080:8080
```

The base runs the deterministic fake engine. It creates no secret and requests no GPU.

## GKE GPU overlay — billable opt-in

Applying this overlay starts one L4-backed vLLM pod and continues billing until the workload and GPU node capacity are removed. It is not a scale-to-zero path.

Before applying it:

1. Build the gateway image, push it to Artifact Registry, and set that exact image in the overlay:

   ```bash
   cd deploy/k8s/gke
   kustomize edit set image sloserve-gateway=REGION-docker.pkg.dev/PROJECT/REPOSITORY/sloserve-gateway@sha256:IMAGE_DIGEST
   ```

2. Create the untracked cache-salt file:

   ```bash
   printf 'cache-salt-secret=%s\n' "$(openssl rand -hex 32)" > secrets.env
   ```

3. Confirm the cluster has an L4 node pool and NVIDIA drivers, then inspect the rendered resources:

   ```bash
   kubectl kustomize .
   kubectl apply -k .
   ```

`podmonitoring.yaml` is optional and requires Google Managed Service for Prometheus collection. The KEDA example is also opt-in: install KEDA, apply `prometheus.yaml`, then apply `keda-vllm.example.yaml`. That example can scale from one to two L4 replicas and therefore raises the cost ceiling; it is intentionally excluded from `kustomization.yaml`.

Delete the GPU workload as soon as the bounded experiment ends:

```bash
kubectl delete -k deploy/k8s/gke
```
