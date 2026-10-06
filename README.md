# Estate Agency

Branded sales-only estate agency website and admin for a small team. Built with Go, server-rendered templates and bbolt.

## Run

```sh
ADMIN_PASSWORD=change-me go run ./cmd/server
```

Open http://localhost:8080. The admin area is at `/admin` (HTTP basic auth). Demo listings are seeded on first run.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `ADDR` | `:8080` | Listen address |
| `DATA_DIR` | `./data` | bbolt database and uploaded photos |
| `BASE_URL` | `http://localhost:8080` | Public URL, used for photo links sent to Rightmove |
| `ADMIN_USER` | `admin` | Admin username |
| `ADMIN_PASSWORD` | (required) | Admin password |
| `SEED_DEMO` | `true` | Seed demo data into an empty database |
| `RIGHTMOVE_MODE` | `dryrun` | `dryrun`, `test` or `live` |
| `RIGHTMOVE_NETWORK_ID` | | Network ID issued by Rightmove |
| `RIGHTMOVE_BRANCH_ID` | | Branch ID issued by Rightmove |
| `RIGHTMOVE_CERT_FILE` | | RTDF client certificate (PEM) |
| `RIGHTMOVE_KEY_FILE` | | RTDF client key (PEM) |
| `RIGHTMOVE_API_BASE` | | Override the RTDF endpoint |

In `dryrun` mode, Rightmove payloads are built and logged but not sent.

## Test

```sh
go test ./...
```
