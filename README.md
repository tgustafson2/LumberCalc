# LumberCalc

LumberCalc finds the cheapest set of boards to buy for a project.

The API is Go, in `backend/`. The web app is Vite and React, in `frontend/`.

## Local only

LumberCalc runs on your computer. It uses a local PostgreSQL database, the Go API on port 8080, and the Vite dev server on port 5173.

The project does not use Docker, Compose, Redis, Terraform, or AWS. Do not add them.

Sign-in uses Clerk. Clerk is a hosted service. You need a network connection to sign in.

## Requirements

You need these tools.

- Go 1.24 or later. `backend/go.mod` says `go 1.24.3`.
- pnpm. The version is the `packageManager` field in `frontend/package.json`.
- PostgreSQL.
- A Clerk development application. Add `http://localhost:5173` as an allowed origin.

## Make the database

Start PostgreSQL. Then make a database with the name `lumbercalc`.

```bash
createdb lumbercalc
psql lumbercalc -c 'select 1'
```

The `psql` command must show one row.

## Set the secrets

Copy the two example files.

```bash
cp backend/.env.example backend/.env
cp frontend/.env.example frontend/.env
```

Edit the two new files.

- Set `DATABASE_URL` in `backend/.env`. The example value is `postgres://localhost:5432/lumbercalc?sslmode=disable`.
- Set `CLERK_SECRET_KEY` in `backend/.env`. The key starts with `sk_test_`.
- Keep `CLERK_AUTHORIZED_PARTIES` in `backend/.env` set to `http://localhost:5173`. Sign-in fails without it.
- Set `VITE_CLERK_PUBLISHABLE_KEY` in `frontend/.env`. The key starts with `pk_test_` or `pk_live_`.

Use the two keys from the same Clerk application.

Git ignores `backend/.env` and `frontend/.env`. Do not commit them. Do not put a real key in an `.env.example` file.

## Apply the migrations

```bash
cd backend
go run ./cmd/migrate
```

The command prints `applied` and the migration name for each new migration. If the database is current, the command prints `up to date`.

## Start the API

Use a new terminal. Run the command in `backend/`.

```bash
cd backend
go run ./cmd/api
```

The API reads `.env` from the current directory. In a different directory, the API stops with these errors.

- `DATABASE_URL is required`
- `CLERK_SECRET_KEY is required`
- `CLERK_AUTHORIZED_PARTIES is required`

Make sure that the API is ready.

```bash
curl http://localhost:8080/readyz
```

`status` is `ready`. The `postgres` check and the `migrations` check have `ok` true. If the `migrations` check is not ok, apply the migrations again.

## Start the web app

Use a new terminal.

```bash
cd frontend
pnpm install
pnpm dev
```

Open `http://localhost:5173` and sign in. The dev server sends each `/v1` request to the API on port 8080.

## If sign-in fails

The page shows a key error. Make sure that `VITE_CLERK_PUBLISHABLE_KEY` starts with `pk_test_` or `pk_live_`.

Sign-in works, but the app cannot load your account. Make sure that `CLERK_AUTHORIZED_PARTIES` is `http://localhost:5173`. Make sure that the two keys come from the same Clerk application.

## Rules for contributors

`AGENTS.md` has the code rules, the test commands, and the code generation commands. `docs/architecture/` has the backend and frontend layers.
