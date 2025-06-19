# Agents

This repository contains several small helper components used to collect data from an Idena node, outside of the main web server. These agents are primarily for data collection, bootstrapping, or monitoring tasks and are optional in typical setups.

## identity_fetcher

The **identity_fetcher** (`agents/identity_fetcher.go`) polls a specific list of addresses and writes each address’s latest identity state to a JSON file (a “snapshot”). It uses a simple JSON config file with the following fields:

- `interval_minutes` – how often (in minutes) to poll the addresses
- `node_url` – RPC URL of your Idena node (e.g. `http://localhost:9009`)
- `api_key` – *(optional)* API key for your Idena node (if it requires one)
- `address_list_file` – path to a text file containing the addresses to query (one address per line)

An example configuration is provided in `agents/fetcher_config.example.json`. To use the fetcher agent:

```bash
# from the project root
cp agents/fetcher_config.example.json agents/config.json
nano agents/config.json   # (or edit the file with your preferred editor)
```

Update the agents/config.json fields to match your environment (at minimum, the node_url and possibly api_key, plus the input/output file paths). Then run the agent:

```bash
cd agents
go run identity_fetcher.go agents/config.json
```

This will periodically contact your Idena node to get the identity information for each address in your list (using a public fallback API for any addresses not recognized by your node). The snapshot is written to `data/whitelist_epoch_<N>.json`, where `<N>` is the current epoch.

    Usage Note: In the context of IdenaAuthGo, running this agent was originally a way to feed data into the whitelist system before the rolling indexer existed. Now that the rolling indexer is available and actively maintained, you typically do not need to use the identity_fetcher for the main whitelist – the indexer will gather all identities automatically. However, the fetcher can still be useful for one-off data collection or to quickly bootstrap the database with a known set of addresses. (The main application can read the generated snapshot file if configured to do so, but by default it expects the rolling indexer to be running.) 

Advanced: There is also a convenience wrapper cmd/agents.go (with its own config in config/agents.json) that can run the identity fetcher. This isn’t usually needed unless you are integrating multiple agents in one process.

### session_block_finder

The session_block_finder (agents/session_block_finder.go) monitors your Idena node for the start of validation sessions. It detects when the Short Session and Long Session begin during the Idena validation ceremony by watching the node’s blockchain flags.

Like the fetcher, it uses a JSON config (see agents/session_finder_config.example.json for the format). The config typically includes your node’s RPC URL and API key (if needed). To run the session finder:

```bash
cp agents/session_finder_config.example.json agents/session_config.json
# (edit agents/session_config.json if your node URL or API key differ)
cd agents
go run session_block_finder.go agents/session_config.json
```

The tool will continuously poll the node’s bcn_lastBlock or bcn_block endpoint until it sees the ShortSessionStarted event, and then the LongSessionStarted event. Once detected, it prints out:

    The block height at which the Short Session started.

    The block height at which the Long Session started.

    The range of blocks corresponding to the short answer submission window (usually this is a 6-block range immediately following the short session start).

This agent is mainly useful for logging or debugging, to know exactly when the validation ceremony sessions occur. It doesn’t directly interact with the main application, but it can help you time or verify certain off-chain processes if needed.

## Rolling Indexer

The rolling indexer (rolling_indexer/main.go) is a lightweight indexing service that maintains a rolling history (about 30 days) of all Idena identities. It continuously pulls identity data from your Idena node and updates a local database, then provides an API to query that data.

    Data Storage: By default it stores identities in an SQLite database file (rolling_indexer/identities.db). No external database (like PostgreSQL) is required.

    History Window: The indexer focuses on recent data (last few epochs). This keeps the database small and queries fast. It’s not meant for full archival of all past identities, but rather to support current and recent whitelist computations.

    HTTP API: The indexer runs its own HTTP server (default port 8080) exposing endpoints such as /identities/latest, /identities/eligible, /identity/{address}, etc., which return JSON data about identities. (See the README for a detailed list of these endpoints.)

Configuration: You can configure the indexer via a JSON file or environment variables:

    RPC_URL – Idena node RPC endpoint URL (point this to your node, e.g. http://localhost:9009).

    RPC_KEY – (optional) API key for your Idena node’s RPC, if needed.

    FETCH_INTERVAL_MINUTES – polling frequency (in minutes). For example, 10 means the indexer will refresh its data every 10 minutes.

    DB_PATH – path to the SQLite DB file (if you want it somewhere other than the default identities.db).

    USE_PUBLIC_BOOTSTRAP – (bool) if true, on first run the indexer will fetch a snapshot of identities from recent past epochs using a public API. This helps populate the database immediately even if your node was not running in those past epochs.

    BOOTSTRAP_EPOCHS – number of past epochs to fetch when bootstrapping (e.g. 3 for roughly the last three epochs).

To build and run the indexer:

```bash
cd rolling_indexer
go build -o rolling-indexer main.go   # build the binary
./rolling-indexer                    # run it (with config via file or env as described)
```

Once running, the rolling indexer will continuously sync identity data from your node. The main IdenaAuthGo web server leverages this data for its whitelist and eligibility computations. In a typical deployment, you will run both the web server (on port 3030) and the rolling indexer service (on port 8080) concurrently.

    Note: If you previously set up the official Idena indexer (the separate idena-indexer project that uses PostgreSQL), you can consider that a legacy approach for this application. The built-in rolling indexer is now the recommended solution. It simplifies the setup by using SQLite and requires no external DB. The rolling indexer ensures you have the necessary identity data in real-time for the whitelisting process. It even falls back to a public API for any identities your node might have missed (the addresses for which such fallback is allowed are listed in rolling_indexer/addresses.txt). In short, you do not need a PostgreSQL database or the external indexer service to use IdenaAuthGo’s latest features – just run the rolling indexer alongside your Idena node.
