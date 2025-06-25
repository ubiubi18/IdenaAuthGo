# IdenaAuthGo

This project is a Go backend for verifying Idena identities and building whitelists. It provides implementations of a "Login with Idena" flow and a lightweight identity indexer, among other features.  
⚠️ **Use at your own risk** – the code is under active development and has not been audited.

## Current Features

- **Sign in with Idena:** Implements the deep-link flow (`/signin` and `/callback`) to authenticate users via the Idena mobile app.
- **Eligibility Check:** Evaluates an identity’s status and stake against Proof-of-Humanity criteria. Humans must meet the dynamic *discrimination stake threshold* defined by the network, while Verified or Newbie identities need at least **10,000 iDNA** stake to qualify.
- **Whitelist Endpoints:** Provides whitelist data for the current or past epochs. 
  - `/whitelist/current` – returns the whitelist of eligible addresses for the current epoch  
  - `/whitelist/epoch/{epoch}` – returns the whitelist for a specified epoch  
  - `/whitelist/check?address=...` – checks a single address’s inclusion and eligibility status
- **Eligibility Snapshot:** `/eligibility?address=...` shows an address’s eligibility as of the snapshot block/epoch and predicts its status for the next epoch.
- **Penalty Exclusion:** Automatically excludes any identity with a validation *Penalty* in the current epoch from the whitelist.
- **Merkle Tree Proofs:** `/merkle_root` returns the Merkle root of the current whitelist. `/merkle_proof?address=...` returns a Merkle proof for a given address (if that address is in the current whitelist).
- **Identity Indexer (Rolling):** A built-in indexer under `rolling_indexer/` continuously polls identity data from an Idena node, stores it in a local SQLite database, and exposes a REST API for identity queries. *(This replaces the need for the external Idena indexer service.)*
- **Agent Scripts:** Utility scripts under `agents/` (for example, an `identity_fetcher` and a `session_block_finder`) help with data collection and monitoring. These are primarily for bootstrapping or debugging and are optional in normal operation.
- **Admin Tools:** Experimental React interface for custom eligibility scripting, batch address checks, and webhook integrations. All custom scripts run locally in the browser.

## Setup & Usage

### 1. Prerequisites

- Install [Go 1.20+](https://go.dev/dl/) and SQLite3.
- Ensure you have access to an **Idena node** (the official client) that is fully synchronized. The indexer will connect to this node via RPC to retrieve identity data. By default, a local Idena node API is available at `http://127.0.0.1:9009` (adjust if using a remote node or different port).

### 2. Clone the Repository

```bash
git clone https://github.com/ubiubi18/IdenaAuthGo.git
cd IdenaAuthGo
```

### 3. Configure Environment

Create and edit the environment file:

```bash
cp .env.example .env
```

Set the necessary values in .env (or as actual environment variables):

```
BASE_URL – the base URL for your running backend (e.g. http://localhost:3030)

IDENA_RPC_KEY – (optional) your Idena node’s API key, if your node requires one for RPC calls
```

For example, on a Unix-like system you can export them directly:

```bash
export BASE_URL="http://localhost:3030"
export IDENA_RPC_KEY="your_idena_node_api_key"
```

Note: The IDENA_RPC_KEY is only needed if your Idena node’s API is protected by a key. If the node’s HTTP API is open or uses default settings on localhost, you can omit this.

The Idena node expects the API key to be included in each JSON-RPC request as a `key` field inside the JSON body. HTTP headers such as `Authorization` or `api-key` are ignored. Example:

```bash
curl -H "Content-Type: application/json" \
  -d '{"method":"bcn_lastBlock","params":[],"id":1,"key":"YOUR_API_KEY","jsonrpc":"2.0"}' \
  http://127.0.0.1:9009
```

Do not send the key in an HTTP header; it must be part of the JSON-RPC payload.

### 4. Run the Web Server (Main API)

Use Go to run the main server:

```bash
go run main.go
```

This will start the IdenaAuthGo backend on port 3030 (listening at the BASE_URL you configured, e.g. http://localhost:3030). Once running, the following HTTP endpoints are available (on port 3030):

```
/signin – Initiates the “Login with Idena” process (generates a deep link that the Idena app can open).

/callback – Handles the callback from the Idena app after the user signs the authentication request.

/whitelist/current – Returns the whitelist of eligible addresses for the current epoch (JSON array of addresses and associated info).

/whitelist/epoch/{epoch} – Returns the whitelist for a specific past epoch.

/whitelist/check?address=<addr> – Checks a single address and returns whether it’s eligible and on the current whitelist (along with details like its identity status and stake).

/eligibility?address=<addr> – Returns the eligibility status of the given address as of the snapshot (whether it meets the criteria or if it’s excluded due to penalty, etc.), and if possible, predicts eligibility for the upcoming epoch.

/merkle_root – Returns the Merkle root of the current epoch’s whitelist.

/merkle_proof?address=<addr> – Returns a Merkle proof for the given address confirming its inclusion in the current whitelist (or an error if not included).
```

Example usage: To check an address’s eligibility from the command line, you can use curl:

```bash
curl "http://localhost:3030/eligibility?address=0xYourAddressHere"
```

### 5. Build & Run the Rolling Indexer (Identity Indexer Service)

For full functionality, you should run the rolling indexer service in parallel with the main web server. The indexer connects to your Idena node to gather identity data continuously.

First, build the indexer binary:

```bash
cd rolling_indexer
go build -o rolling-indexer main.go
```

Next, configure the indexer. You can use a JSON config file or environment variables to specify how it connects to your node:

```
RPC_URL – URL of your Idena node’s RPC endpoint (e.g. http://localhost:9009 for a local node).

RPC_KEY – API key for your Idena node, if it requires one.

FETCH_INTERVAL_MINUTES – Polling interval in minutes (how often to query the node for updates).

USE_PUBLIC_BOOTSTRAP – If set to true, the indexer will fetch historical identity data on first startup (from a public source) to populate recent past epochs. This is useful if your node has not been running long, so you can catch up on identities you might have missed.

BOOTSTRAP_EPOCHS – If bootstrapping is enabled, how many past epochs of identities to fetch initially (e.g. 3 will retrieve roughly the last three epochs of data).
```

You can put these in a rolling_indexer/config.json file or export them as environment variables. For example, to run with environment variables:

```bash
# From the IdenaAuthGo/rolling_indexer directory:
export RPC_URL="http://localhost:9009"          # your Idena node RPC URL
export RPC_KEY="your_idena_node_api_key"        # your node’s API key (if needed)
export FETCH_INTERVAL_MINUTES=10               # poll every 10 minutes
export USE_PUBLIC_BOOTSTRAP=true               # enable one-time bootstrap of past data
export BOOTSTRAP_EPOCHS=3                      # fetch the last 3 epochs on first run

./rolling-indexer
```

When the indexer runs, it will create (or use) an SQLite database file at rolling_indexer/identities.db to store identity snapshots. By default, it listens on port 8080 and provides its own HTTP API for identity data queries. Key endpoints exposed by the indexer (on port 8080) include:

```
/identities/latest – Returns a snapshot of all identities and their latest known state (at the last update cycle).

/identities/eligible – Returns only the identities (addresses) that are currently eligible for Proof-of-Humanity (i.e. those that meet the criteria: correct identity status and sufficient stake, no validation penalty).

/identity/{address} – Returns the full history of identity states for the given address (all snapshots recorded in the 30-day window).

/state/{IdentityState} – Returns all addresses currently in the given identity state (e.g. Human, Verified, Newbie, etc., as recognized by the Idena protocol).
```

You can test the indexer service independently, for example:

```bash
curl http://localhost:8080/identities/eligible
```

This should return a JSON array of addresses that the indexer currently deems eligible.

Important: The rolling indexer is the primary data source for identity information in IdenaAuthGo. The main web server will use the data collected by this service (either via direct database access or via HTTP calls) to respond to whitelist and eligibility queries. Ensure the indexer is running and synced with your node, especially in production, so that the eligibility calculations are based on up-to-date data. Running this built-in indexer replaces the need for any external or “official” Idena indexer service. In other words, you do not need to run the official idena-indexer (which requires a PostgreSQL database) for this project – the rolling indexer covers all necessary functionality using SQLite.

### 6. (Optional) Identity Fetcher Agent

You can skip this step if you’re running the rolling indexer. This agent is mainly for specialized use cases or initial data seeding.

The identity_fetcher agent (`agents/identity_fetcher.go`) polls your Idena node for the identity details of a set of addresses and writes the results to a JSON snapshot file. By default the address list is obtained from the rolling indexer (`/api/whitelist/current`), so no manual list is required. You can still provide a static list via the `-address-file` flag if needed for special cases.

To use it:

Prepare the configuration:

```bash
cp agents/fetcher_config.example.json agents/config.json
```

Open `agents/config.json` in an editor and set the node connection fields (RPC URL and API key). The indexer URL can also be customised but defaults to `http://localhost:8080`.

Run the fetcher:

```bash
go run cmd/fetcher/main.go -config agents/config.json
```

The command automatically queries your node for the current epoch and writes
`data/whitelist_epoch_<N>.json` (where `<N>` is the epoch number). Run this
periodically – for example via cron – to keep the snapshot fresh. If you’re
running the rolling indexer you can skip this step, as the indexer provides the
same data automatically.

Example hourly cron entry:

```
0 * * * * cd /path/to/IdenaAuthGo && /usr/local/go/bin/go run cmd/fetcher/main.go -config agents/config.json >> fetcher.log 2>&1
```

### 7. (Optional) Session Start Block Finder

The `session_block_finder` agent (`agents/session_block_finder.go`) helps determine when the Idena validation ceremony sessions start, by watching for specific blockchain flags. It’s useful for logging or monitoring purposes.

To use this tool:

Copy the example config:

```bash
cp agents/session_finder_config.example.json agents/session_config.json
```

(Adjust the config if needed – by default it may just need the node’s URL and API key.)

Run the agent:

```bash
cd agents
go run session_block_finder.go agents/session_config.json
```

This will continuously poll your node’s block API until it detects the `ShortSessionStarted` and `LongSessionStarted` events. When detected, it will print out the block height at which the Short Session started, the block height at which the Long Session started, and the range of blocks that correspond to the Short Session (this range is typically 6 blocks long, as the short answers window is very brief).

You can run this before a validation ceremony to know exactly when the sessions commence.

### 8. Exporting the Merkle Root (Manual Snapshot)

The IdenaAuthGo server automatically rebuilds the whitelist and computes a new Merkle root when it detects that a new epoch has begun (i.e. after a validation ceremony). This ensures that `/whitelist/*` and `/merkle_root` always reflect the latest epoch’s data once your node and indexer are updated.

If you need to manually trigger a whitelist snapshot and Merkle tree computation (for example, for testing or forcing an update), you can run the server in a special mode:

```bash
go run main.go -index
```

This will not start the web server; instead, it will fetch the latest identity data (using the rolling indexer’s database or directly from the node RPC) and generate a fresh whitelist snapshot. The resulting whitelist will be saved to the `data/` directory as `whitelist_epoch_<N>.json` (where `<N>` is the current epoch number), and the Merkle root for that list will be printed to the console. The same Merkle root will be served by the `/merkle_root` endpoint, and `/merkle_proof?address=...` will provide inclusion proofs for addresses on the list.

### Disclaimer

This project is provided as-is for experimental, non-commercial use. No warranties or guarantees are given regarding its functionality, security, or performance. Use of IdenaAuthGo is at your own risk. The maintainers and contributors are not liable for any damages or losses resulting from running this software. Always review and test the code in your environment before using it in production.
