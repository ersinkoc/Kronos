# GKE Overlay

Use this overlay as a starting point for Google Kubernetes Engine clusters:

```bash
kubectl apply -k deploy/kubernetes/overlays/gke
```

Before applying in production, replace the Workload Identity service account
placeholder in `serviceaccount.yaml` and pin the Kronos image to an immutable
digest.
