# Nối từ backend

Local setup: copy `.env.example` to `.env` and fill in the database URL; `.env` is ignored by Git. The Go process loads `.env` automatically when an environment variable is not already set, so `go run .` works from the repository root. Run `make run` to start the API on `127.0.0.1:8080`, `make test` for unit tests, or `make test-db` for integration tests against the configured database. The schema must already exist, or can be initialized using the command below. Stop the process with Ctrl+C.

For a public/VPS bind such as `ADDR=:8080`, configure `API_KEY` or `API_KEY_FILE`; the process refuses to start publicly without an API key. `DATABASE_URL_FILE` and `API_KEY_FILE` are supported for Docker secrets. API-key protected endpoints accept `X-API-Key` (or `Authorization: Bearer ...`). Set `COLLECT_AUTOSTART=1` only when an automatic start after restart is desired; the Swarm stack defaults it to `0`, so the extension/user controls the cron.

Open `http://127.0.0.1:8080/swagger` to view and try the API in Swagger UI. The raw OpenAPI document is at `http://127.0.0.1:8080/openapi.json`. Use `POST /collector/toggle` with `{"enabled":true}` to start the collector schedule or `{"enabled":false}` to stop it at runtime. The first run starts immediately; later runs follow `COLLECT_INTERVAL_MINUTES` (default 30 minutes). `POST /settings/suggestions` controls the suggestion feature, while `GET /settings` returns both states. These two toggle states are persisted in PostgreSQL, so a Swarm restart does not silently re-enable a disabled cron.

For the collector, put a valid `NOITU_ACCESS_TOKEN` from your own authenticated noitu.fun session in `.env` and run `make collect` for one game or `make collect-scheduled` for the foreground scheduler (30 minutes between completed games; stop with Ctrl+C). You can override the interval by setting `COLLECT_INTERVAL_MINUTES` before running `make collect-scheduled`. A missing or expired token stops the collector; obtaining and refreshing it remain manual. Do not paste tokens into chat or commit them.

Alternatively, install the unpacked `extension` folder through Chrome's Extensions page (enable developer mode, choose Load unpacked). Open `https://www.noitu.fun/mot-minh` or an online game in that Chrome profile and complete any verification normally. Open the extension popup, set the HTTPS VPS backend URL and API key, then save. The extension reads that site's guest access token, refreshes it through the site's normal refresh endpoint when needed, and sends it over HTTPS to `POST /collector/token`; the VPS keeps it in memory for its collector. The popup can refresh status and enable/disable both suggestions and cron. Keep the game tab open so the extension can refresh and relay tokens. The local `127.0.0.1:8090` bridge remains available only when `COLLECT_BROWSER=1` for compatibility.

The same extension watches the current word in both solo and online game tabs. It sends normalized game words through the background service worker to `POST /session/suggestions` on the configured backend and shows ranked suggestions in a small panel. The backend records those words as accepted observations asynchronously. API keys are never sent through the page DOM; the service worker adds them to backend requests.

## Docker Swarm deployment

Build and push the image to a registry reachable by every Swarm node:

```bash
export NOITU_IMAGE=registry.example.com/your-user/noitu:1.1.0
docker buildx build --platform linux/amd64 -t "$NOITU_IMAGE" --push .
```

Create Docker secrets on the Swarm manager. The database URL can point to the existing PostgreSQL server or to a private database service:

```bash
printf '%s' 'a-long-random-api-key' | docker secret create noitu_api_key -
printf '%s' 'postgres://user:password@db.example:5432/postgres?sslmode=require' | docker secret create noitu_database_url -
NOITU_IMAGE="$NOITU_IMAGE" docker stack deploy -c docker-stack.yml noitu
```

Put the public HTTPS reverse-proxy URL of the service into the extension popup. Do not expose PostgreSQL or port 8090 publicly. Put TLS and rate limiting in the reverse proxy, and restrict `ALLOWED_ORIGINS` to the exact game site origin.

Requires Go and PostgreSQL. Apply `schema.sql`, set `DATABASE_URL`, then run `go run .`. Optional `ADDR` defaults to `:8080`.

`GET /suggest?key=bình` streams SSE events: `suggestion` contains `word` and `verified`; `source_error` signals the external dictionary failed; `done` ends the stream. Observed accepted words arrive first, followed by unverified two-syllable words from Vua Nối Từ. Previously rejected words are excluded. The external dictionary is loaded once per process and may be stale until restart.

After individual suggestions, `ranking` contains the complete sorted array with `opponent_replies`, `worst_next_moves`, and `assessment`. Add repeated `used` query parameters, such as `?key=bình&used=bình%20minh`, to exclude words already played. The ranking examines the opponent's next move and our following move, assuming both sides draw from the available two-syllable dictionary. It ranks potential kills first, then moves with at least one known response to every known reply, then risky moves; within a category verified words come first. `possible_kill` and `has_reply` are estimates, not guarantees: unverified candidates may be rejected and the dictionary is incomplete. `worst_next_moves` is -1 when no opponent reply is known. Use the final `ranking` event to order the UI; early `suggestion` events are provisional.

`POST /observations` accepts `{"phrase":"bình minh","accepted":true}`. Send an observation only when the actual game result establishes whether the phrase exists; failures caused by turn, repetition, timeout, or connection are not dictionary rejections. This endpoint trusts its caller; restrict access before deploying publicly. The latest observation wins, and verified means previously accepted, not guaranteed valid in the current game.

Observations use a bounded in-memory queue and HTTP 202 means queued, not persisted. A process crash or database failure can lose queued observations; logs report write failures. Use a durable queue if loss is unacceptable. The dictionary belongs to its owner; verify permission and licensing before reuse or redistribution.

Run `go test ./...`.

For PostgreSQL integration tests, set `TEST_DATABASE_URL` and run `go test -race ./...`. They use a temporary table, which disappears when the connection closes. To initialize the persistent schema, run `INIT_SCHEMA=1 go test -run TestInitializeSchema`; the test leaves an existing table unchanged.

To run one experimental solo collector, set `DATABASE_URL`, `NOITU_ACCESS_TOKEN` to a valid token obtained through noitu.fun's normal authentication, and `COLLECT_ONCE=1`, then run `go run .`. Alternatively, set `COLLECT_INTERVAL_MINUTES` to at least 30 to repeat in the foreground. The collector uses at most 10 answers separated by 3 seconds, stops on the first rejected answer, and synchronously saves only words returned or accepted by the game. It does not bypass Turnstile or record ambiguous failures as invalid. The extension refreshes and relays the guest token, but the collector still stops when no valid token is available. This unofficial integration may stop working if noitu.fun changes its API; use it only where the site's terms and permissions allow.
