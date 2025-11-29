Build/run:
- `make run` (dev server with configs/example.yaml)
- `make dev-up` / `make dev-run` / `make dev-all` (local deps + app)
- `make debug-up` / `make debug-run` / `make debug-all` (debug containers + app)
- `make compose-up` / `make compose-down` (docker compose stack)
- `make build` / `make build-linux` (binaries)

Quality:
- `make fmt` or `make fmt-check`
- `make vet`
- `make lint`
- `make test` / `make test-quick` / `make test-verbose` / `make test-coverage`
- `make test-all` or `make test-ci` (scripted full suite)

Docs & tools:
- `make swagger-init` then `make swagger-gen` or `make api-docs` (Swagger)
- `make install-hooks` (pre-commit gofmt check)

Deploy/ops:
- `make deploy` (test deploy), `make deploy-full` (with backup)
- `make quick-deploy` / `make auto-deploy`
- `make docker-build` / `make docker-push`
- `make backup` / `make restore`
- `make clean` / `make clean-all`

Observability:
- HTTP health: `/healthz`, `/readyz`; metrics at `/metrics` (per README).
- Logs/helpers: `make prod-logs`, docker compose logs commands from README/Makefile.