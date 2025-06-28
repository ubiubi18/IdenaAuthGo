# Strict Whitelist Server

This lightweight server exposes endpoints for building a strict whitelist directly from a local Idena node. It serves a static web page from `static/` at the root path.

## Usage

Set the environment variable `IDENA_RPC_KEY` if your node RPC is protected by an API key. The value must **not** be hardcoded in your code or logs. Start the server:

```bash
go run ./cmd/strictserver
```

### Endpoints

- `/snapshot` &mdash; build a new whitelist by fetching identities from the node
- `/whitelist/current` &mdash; serve the most recently generated whitelist
- `/whitelist/epoch/{n}` &mdash; serve the whitelist for epoch `n`
- `/whitelist/check?address=ADDR` &mdash; return eligibility info for `ADDR`
- `/whitelist/download` &mdash; download the latest whitelist as `whitelist.jsonl`

See the example commands in `static/index.html` for quick testing.

You can replace `static/index.html` with your own file to customize the landing page.

### Requirements

- A running Idena node with RPC enabled (default `http://localhost:9009`).
- `IDENA_RPC_KEY` exported if the node requires an API key.
