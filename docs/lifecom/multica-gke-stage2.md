# Multica GKE Stage 2 — Self-host to Production

**Owner**: 來富 / Eric
**Domain**: `ai.licodes.net`
**Date**: 2026-05-08

## Decisions (locked)

| # | Decision | Choice |
|---|---|---|
| 1 | Image build | Cloud Build 2nd-gen (consistent with LifeCOM infra) |
| 2 | Postgres | GKE StatefulSet (`pgvector/pgvector:pg17`), PVC-backed; defer CloudSQL until data volume justifies |
| 3 | Domain & LB | Reuse existing `laravel-gateway` (IP `34.8.116.67`); add HTTPRoute for `ai.licodes.net` |
| 4 | Migration source | Re-export from SaaS workspace `4675fba6-6acd-42e3-88fd-04fa0471480f` (skip WSL POC data) |
| 5 | Namespace | `multica` (dedicated) |

## Architecture

```
DNS ai.licodes.net ─→ 34.8.116.67 (existing GCP LB)
                           │
                       laravel-gateway (Gateway API)
                           │
              ┌────────────┴────────────┐
              │                         │
        HTTPRoute (existing)      HTTPRoute (NEW)
        host=laravel.xxx          host=ai.licodes.net
              │                         │
        ns: laravel-app          ns: multica
              │                  ┌──────┴──────┐
        laravel-svc          multica-web   multica-backend
                                   :3000        :8080
                                                   │
                                          multica-postgres (StatefulSet, pgvector)
                                                   :5432  + PVC
```

## Stage 2 Steps

### S1. GAR repository + Cloud Build trigger
- Create Artifact Registry repo `multica` in region `asia-east1`
- Cloud Build trigger: source = GitHub `EricSu-Work/multica` branch `main-lifecom`, build via repo's `Dockerfile.backend` + `apps/web/Dockerfile`
- Push tags: `multica-backend:<sha>`, `multica-web:<sha>` and `:latest`

### S2. Namespace + secrets
- `kubectl create ns multica`
- Secret `multica-env`:
  - `JWT_SECRET` (regenerate, NOT WSL POC's)
  - `DATABASE_URL=postgres://multica:<pw>@multica-postgres:5432/multica?sslmode=disable`
  - `APP_ENV=production`
  - `ALLOW_SIGNUP=false` (prod hardening)
  - `RESEND_API_KEY` (TBD — or leave unset and email codes go to log)
- ServiceAccount + Workload Identity if backend needs GCP API access (probably not for v1)

### S3. Postgres StatefulSet
- Image: `pgvector/pgvector:pg17`
- PVC: 20Gi, `pd-balanced`
- Init job runs Multica's migration suite (`server/pkg/db/migrations/`)
- Internal Service: `multica-postgres.multica.svc:5432`

### S4. Backend + Frontend Deployments
- backend: 1 replica, requests 500m CPU / 512Mi
- frontend: 1 replica, requests 200m CPU / 256Mi
- Internal Services: `multica-backend:8080`, `multica-web:3000`
- Frontend env: `NEXT_PUBLIC_API_URL=https://ai.licodes.net` (same-origin)
- Backend env: `FRONTEND_ORIGIN=https://ai.licodes.net`

### S5. HTTPRoute on laravel-gateway
- Apply `HTTPRoute` in `multica` ns:
  - parentRef: `laravel-gateway` (cross-ns; ensure ReferenceGrant or grant via Gateway listener AllowedRoutes)
  - hostnames: `ai.licodes.net`
  - rules:
    - `path: /api/**` → `multica-backend:8080`
    - `path: /auth/**` → `multica-backend:8080`
    - `path: /ws` → `multica-backend:8080` (WebSocket)
    - `path: /` (catch-all) → `multica-web:3000`
- TLS: managed cert via Gateway's existing cert config (verify Gateway has TLS listener that includes `*.licodes.net` or add SAN)

### S6. Smoke test
- `curl https://ai.licodes.net/health` → `{"status":"ok"}`
- Open `https://ai.licodes.net/` → login screen
- Send-code from prod → grab from `kubectl logs deploy/multica-backend | grep "code for"` (until Resend wired)

### S7. Migrate from SaaS (re-export)
- Re-run `multica-migrate.py export --workspace-id 4675fba6-... --out /tmp/multica-export-prod`
  - (Refresh in case anything changed since 2026-05-08 11:00)
- Login UI on prod → create workspace `LifeCOM` (slug=`lifecom`, prefix=`LIF`)
- Mint PAT via `/api/tokens`
- Patch `~/.multica/config.json` (or use a separate prod profile) → `server_url=https://ai.licodes.net`
- `multica daemon stop && multica daemon start` to register runtime against prod
- Run `multica-migrate.py import --workspace-id <prod-WS-id> --in /tmp/multica-export-prod --runtime-id <prod-rt>`

### S8. Daemon switchover
- Once import verified, this becomes the **production daemon** (Eric's WSL machine)
- Update memory: prod URL, prod workspace ID, prod PAT prefix

### S9. Cleanup
- Stop SaaS daemon permanently
- Decide WSL POC stack fate (keep as staging or `docker compose down -v`)

## Risks / Pitfalls

1. **Cross-namespace HTTPRoute → Gateway**: Gateway API needs `ReferenceGrant` in `laravel-app` ns OR Gateway listener with `allowedRoutes.namespaces.from: All`. Check current Gateway YAML before applying HTTPRoute.

2. **TLS cert for `ai.licodes.net`**: existing managed cert may not cover this hostname. Likely need to add to cert's hostname list and wait for issuance (5-30min). Pre-check via `kubectl get gcpcert` or `kubectl describe gateway`.

3. **Gateway path routing for WebSocket**: Multica daemon talks to backend via WS. HTTPRoute supports WS as a normal HTTP upgrade — verify with `wscat` smoke after deploy.

4. **PR#1352 + PR#1650 e2e on prod**: skip on prod — already verified at A-level on WSL. Re-run only if backend image diff is unexpected.

5. **Daemon `client_version` mismatch**: brew CLI 0.2.15 against fork's HEAD backend works on WSL; ensure same on prod (no changes expected).

6. **Resend API key**: not blocking POC, but production should have email working. Decide later.

## Out of scope (Stage 2)

- Cloud SQL migration
- HA / multi-replica
- Backup automation (pg_dump cron)
- Observability (Prometheus / Grafana / log shipping)
- CDN in front of frontend
