.PHONY: generate

generate:
	cd backend && go generate ./internal/server ./internal/api
	pnpm --dir frontend generate
