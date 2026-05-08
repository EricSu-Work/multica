# Multica GKE Stage 2 — Deploy

Manifests + Cloud Build pipeline for self-hosting Multica on GKE under `ai.licodes.net`.

## Files

| File | Purpose |
|---|---|
| `00-namespace.yaml` | `multica` ns |
| `10-secrets.template.yaml` | **Template** — copy and fill `REPLACE_ME_*` before applying |
| `20-postgres.yaml` | StatefulSet + Service for `pgvector/pgvector:pg17`, 20Gi PVC |
| `30-app.yaml` | `multica-backend` (8080) + `multica-web` (3000) Deployments + Services |
| `40-httproute.yaml` | HTTPRoute attached to `laravel-gateway` (laravel-app ns), `host: ai.licodes.net` + HTTP→HTTPS redirect |
| `cloudbuild.yaml` | Build → push → roll pipeline (Cloud Build 2nd-gen) |

## Bootstrap (one-time, manual)

### 1. Artifact Registry repo
```bash
gcloud artifacts repositories create multica \
  --repository-format=docker \
  --location=asia-east1 \
  --description="Multica self-host images" \
  --project=lifeerp
```

### 2. Cloud Build trigger (manual via console)

Console → Cloud Build → Triggers → Connect Repository
- Region: `asia-east1`
- Repo: `EricSu-Work/multica` (use 2nd-gen GitHub host connection if not already linked)
- Trigger:
  - Name: `multica-main-lifecom`
  - Event: Push to branch
  - Branch: `^main-lifecom$`
  - Configuration: Cloud Build configuration file = `deploy/gke/cloudbuild.yaml`

> The Cloud Build SA needs roles:
> `roles/artifactregistry.writer`, `roles/container.developer`, `roles/iam.serviceAccountUser`.

### 3. Apply secrets (after filling values)
```bash
cp deploy/gke/10-secrets.template.yaml /tmp/multica-secrets.yaml
# edit /tmp/multica-secrets.yaml: replace REPLACE_ME_PG_PASSWORD and REPLACE_ME_JWT_SECRET_64HEX
# ( python3 -c "import secrets; print(secrets.token_hex(32))" generates JWT secret )
kubectl apply -f deploy/gke/00-namespace.yaml
kubectl apply -f /tmp/multica-secrets.yaml
rm /tmp/multica-secrets.yaml
```

### 4. Apply persistent + app manifests
```bash
kubectl apply -f deploy/gke/20-postgres.yaml
kubectl -n multica rollout status statefulset/multica-postgres --timeout=5m

# Wait for first image build to complete in Cloud Build (push to main-lifecom triggers it).
kubectl apply -f deploy/gke/30-app.yaml
kubectl -n multica rollout status deployment/multica-backend --timeout=5m
kubectl -n multica rollout status deployment/multica-web     --timeout=5m
```

### 5. Apply HTTPRoute
```bash
kubectl apply -f deploy/gke/40-httproute.yaml

# Verify it attached to the parent gateway:
kubectl describe httproute multica-route -n multica | grep -i 'parent\|condition'

# Smoke:
curl -sS https://ai.licodes.net/health    # expects: {"status":"ok"}
curl -sSI https://ai.licodes.net/         # expects: 200 OK (frontend)
```

## Migration (S7 from `docs/lifecom/multica-gke-stage2.md`)

After the stack is up:

1. Re-export from SaaS (in case data drifted since 2026-05-08):
   ```bash
   # Use a SaaS-pointing CLI profile or temporarily restore SaaS config.
   python3 ~/dev/multica-recon/multica-migrate.py export \
     --workspace-id 4675fba6-6acd-42e3-88fd-04fa0471480f \
     --out /tmp/multica-export-prod
   ```
2. Login UI on `https://ai.licodes.net/`, create workspace `LifeCOM` (slug=`lifecom`, prefix=`LIF`).
3. Mint a PAT via `/api/tokens` (see `skill: multica-selfhost-cutover`).
4. Patch `~/.multica/config.json`:
   ```json
   {
     "server_url": "https://ai.licodes.net",
     "app_url":    "https://ai.licodes.net",
     "workspace_id": "<NEW-PROD-WS-UUID>",
     "token": "<PROD-PAT>"
   }
   ```
5. `multica daemon stop && multica daemon start` — daemon registers the runtime.
6. `multica runtime list --output json` — capture the runtime id.
7. Run import:
   ```bash
   cd ~/dev/multica-recon
   python3 multica-migrate.py import \
     --workspace-id <NEW-PROD-WS-UUID> \
     --in /tmp/multica-export-prod \
     --runtime-id <PROD-RT-ID>
   ```

## Notes

- The pre-shared cert `licodes-net-cert-v2` (cert-map `laravel-cert-map`) already includes `ai.licodes.net` via entry `ai-cert-map-entry → ai-licodes-cert`. No new cert needed.
- The Gateway listener allows routes from any namespace (`allowedRoutes.namespaces.from: All`), so the HTTPRoute in `multica` ns attaches without a `ReferenceGrant`.
- `RESEND_API_KEY` deliberately unset for now — verification codes print to backend log:
  `kubectl -n multica logs deploy/multica-backend | grep "code for"`
