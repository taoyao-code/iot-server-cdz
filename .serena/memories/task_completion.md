When finishing a task:
- Ensure code formatted (`make fmt`) and linted (`make lint`) if code changed.
- Run appropriate tests: at minimum `make test` (race+cover); for broader changes `make test-all` or `make test-ci`; use `make test-coverage` if coverage needed.
- If protocol/API changes, regenerate docs with `make api-docs` and verify configs.
- For deploy-related work, run `make build` or `make build-linux` and use `make deploy`/`make quick-deploy` as required by environment.
- Verify health endpoints locally (`curl localhost:7055/healthz`/`readyz`) when relevant.
- Keep files in UTF-8; avoid touching unrelated changes; follow conventional commits when preparing git commit (if requested).