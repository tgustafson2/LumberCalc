# Backend architecture

The packages in `AGENTS.md` are the backend layers. Put new code in one of those packages. Add a new package only when none of the existing packages can hold the responsibility.

## Packages

- `backend/cmd/api` starts the HTTP process. It wires config, Clerk, store, the `/v1` handler, and the server. The ready check calls `db.CheckApplied`.
- `backend/cmd/migrate` applies the embedded SQL migrations.
- `backend/internal/config` loads `.env` and the process environment. Process values win. A missing `.env` is not an error.
- `backend/internal/server` is the process HTTP stack. `GET /healthz` and `GET /readyz` are public. The server mounts the `/v1` handler when that handler is set. CORS uses the same list as `CLERK_AUTHORIZED_PARTIES`. An exact match echoes the stored origin. Each response sets `Vary` to `Origin`. An allowed preflight ends before `/v1`. The response has no `Access-Control-Allow-Credentials` header.
- `backend/internal/api` is `/v1`. Every path under `/v1` requires a Clerk session, including unknown paths. A handler parses the request, calls store or Clerk, and writes the response. Keep the handler thin.
- `backend/internal/clerkauth` verifies Clerk session JWTs and loads display names. `azp` must equal a party passed to `New`. A blank `azp` is rejected.
- `backend/internal/store` is the database boundary. `UserID` and `ClerkUserID` are constructed only there.
- `backend/db/migrations` and `backend/db/queries` are the schema and the sqlc inputs.
- `backend/internal/store/internal/q` is generated. Do not edit it.

`backend/openapi.yaml` is the HTTP contract. Tag `probes` selects the models in `backend/internal/server/openapi.gen.go`. Tag `v1` selects the models in `backend/internal/api/openapi.gen.go`. `make generate` writes those files and `frontend/src/api.gen.ts`.

## Dependency direction

`cmd/api` depends on `config`, `clerkauth`, `store`, `api`, `server`, and `db`.

`api` depends on `clerkauth`, `server`, and `store`.

`clerkauth` depends on `config` and `store`.

`store` depends on `store/internal/q`.

`config` and `server` do not import other LumberCalc packages.

## Rules for new code

Put a new `/v1` route in `backend/internal/api`.

Put SQL in `backend/db/queries`. Regenerate `backend/internal/store/internal/q` with `sqlc generate`. `api`, `server`, `clerkauth`, and `config` must not execute SQL.

`api` may call `store`. Only `store` imports `store/internal/q`.

Parse and validate at the HTTP boundary, the config boundary, and the Clerk boundary. Trust those types inside the process.

Do not add a new package unless the responsibility cannot belong to an existing package.

Do not add Docker, Compose, Redis, Terraform, or AWS unless the task asks for them. The current runtime is a local Postgres database and the Go process.
