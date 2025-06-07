# Agents

This repository contains several small components used to collect information from an Idena node.

## identity_fetcher

`agents/identity_fetcher.go` polls a list of addresses and writes their latest identity state to a JSON snapshot file. It uses a simple config file with fields:

- `interval_minutes` – polling interval
- `node_url` – RPC endpoint of your Idena node
- `api_key` – optional node API key
- `snapshot_file` – path to write results
- `address_list_file` – file containing addresses to query

An example config is provided in `agents/fetcher_config.example.json`. Copy it to `agents/fetcher_config.json` and run:

```bash
cd agents
go run identity_fetcher.go fetcher_config.json
```

The root `cmd/agents.go` helper also runs this agent with `config/agents.json`.

## session_block_finder

`agents/session_block_finder.go` monitors your node for the blocks
marking the beginning of the validation ceremony.  It polls
`bcn_lastBlock` and `bcn_block` until the `ShortSessionStarted` and
`LongSessionStarted` flags appear, then prints the block heights and the
range of the short-answer window (typically six blocks).

Run it with a simple config file similar to `agents/session_finder_config.example.json`:

```bash
cd agents
go run session_block_finder.go session_finder_config.json
```

## Rolling Indexer

`rolling_indexer/main.go` keeps a 30‑day rolling history of all identities. It stores data in `identities.db` and serves HTTP endpoints such as `/identities/latest` and `/identities/eligible`.

Configuration can be provided via `rolling_indexer/config.json` (create it if needed) or environment variables `RPC_URL`, `RPC_KEY`, `FETCH_INTERVAL_MINUTES`, and `DB_PATH`.

Run the indexer with:

```bash
cd rolling_indexer
go build -o rolling-indexer
./rolling-indexer
```

Address tracking for fallback API requests is read from `rolling_indexer/addresses.txt`.
