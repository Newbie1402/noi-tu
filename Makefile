.PHONY: run test test-db collect collect-scheduled collect-browser extension-check docker-build swarm-deploy

run:
	@set -a; . ./.env; set +a; go run ./cmd/api

test:
	go test -race ./...

extension-check:
	node --check extension/background.js
	node --check extension/content.js
	node --check extension/popup.js

docker-build:
	docker build -f deployments/Dockerfile -t $${NOITU_IMAGE:-noitu:latest} .

swarm-deploy:
	docker stack deploy -c docker-stack.yml $${STACK_NAME:-noitu}

test-db:
	@set -a; . ./.env; set +a; TEST_DATABASE_URL="$$DATABASE_URL" go test -race -count=1 ./...

collect:
	@set -a; . ./.env; set +a; COLLECT_ONCE=1 go run ./cmd/api

collect-scheduled:
	@set -a; . ./.env; set +a; COLLECT_AUTOSTART=1 COLLECT_INTERVAL_MINUTES=$${COLLECT_INTERVAL_MINUTES:-30} go run ./cmd/api

collect-browser:
	@set -a; . ./.env; set +a; COLLECT_AUTOSTART=1 COLLECT_BROWSER=1 COLLECT_INTERVAL_MINUTES=$${COLLECT_INTERVAL_MINUTES:-30} go run ./cmd/api
