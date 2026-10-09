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
| `RIGHTMOVE_CERT_PEM` / `RIGHTMOVE_KEY_PEM` | | Certificate and key contents, instead of files (for hosting secrets) |
| `RIGHTMOVE_API_BASE` | | Override the RTDF endpoint |

In `dryrun` mode, Rightmove payloads are built and logged but not sent.

## Test

```sh
go test ./...
```

## Deploy (Fly.io)

`fly.toml` runs one machine in London (`lhr`) with a 1 GB volume mounted at `/data` for the database and photos. Keep it to a single machine: bbolt is a single-file database.

```sh
fly launch --no-deploy --copy-config --name <app-name>
fly volumes create data --region lhr --size 1
fly secrets set ADMIN_PASSWORD=<password> BASE_URL=https://<your-domain>
fly deploy
fly certs add <your-domain>
```

For Rightmove, set `RIGHTMOVE_MODE`, `RIGHTMOVE_NETWORK_ID`, `RIGHTMOVE_BRANCH_ID`, `RIGHTMOVE_CERT_PEM` and `RIGHTMOVE_KEY_PEM` with `fly secrets set`.

