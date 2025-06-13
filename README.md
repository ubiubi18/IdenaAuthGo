# IdenaAuthGo

NOT USABLE RIGHT NOW! This project is a **work-in-progress** (WIP) Go backend for verifying Idena identities and building whitelists. It provides partial implementations of a “Login with Idena” flow and an identity indexer, among other features.  
⚠️ **Use at your own risk** – not production-ready, not audited.

## Current Features

- **Sign in with Idena:** Partial implementation of the deep-link flow (`/signin`, `/callback`) to authenticate users using the Idena app.
- **Eligibility Check:** Evaluates identity state and stake (Human, Verified, or Newbie with ≥10,000 iDNA).
- **Whitelist Endpoints:** `/whitelist/current` returns the current epoch whitelist; `/whitelist/epoch/{epoch}` fetches a specific epoch; `/whitelist/check` verifies a single address.
- **Penalty Exclusion:** Addresses with a validation penalty in the current epoch are automatically excluded from the whitelist.
- **Merkle Root Endpoint:** Planned endpoint `/merkle_root` to return the Merkle root of the whitelist (not yet implemented).
- **Identity Indexer:** `rolling_indexer/` polls identity data from an Idena node, stores to SQLite (`identities.db`), and serves JSON over HTTP. (⚠️ currently broken — needs debugging).
- **Agent Scripts:** `agents/identity_fetcher.go` fetches identities by address list (configurable via `agents/fetcher_config.example.json`), useful for bootstrapping indexer data.

## Roadmap & Goals

- **Fix and Run Indexer:** Resolve merge conflicts and logic bugs in `rolling_indexer/main.go`; validate endpoints `/identities/latest`, `/eligible`, etc.
- **Feed Identity Data:** Use agent scripts or direct RPC calls to populate the identity indexer database.
- **Build Merkle Tree Generator:** Create the `/merkle_root` endpoint that returns a SHA256-based Merkle root of eligible addresses.
- **Apply Eligibility Criteria:** Ensure consistent rules (state ∈ {Human, Verified, Newbie} && stake ≥ 10,000) across frontend and backend.
- **Update `AGENTS.md`:** Either populate with actual working agents or simplify it to reflect current usage only.
- **Code Cleanup & Tests:** Add tests, remove stale comments/conflicts, and improve error handling.

## Setup & Usage

### 1. Prerequisites

Install [Go 1.20+](https://go.dev/dl/) and SQLite3.

### 2. Clone the Repo

git clone https://github.com/ubiubi18/IdenaAuthGo.git
cd IdenaAuthGo

### 3. Configure Environment

 Copy and edit `.env`:

cp .env.example .env
 Then edit .env to set:
 BASE_URL=http://localhost:3030
 IDENA_RPC_KEY=your_idena_node_api_key (optional)

 Or set environment variables manually:

export BASE_URL="http://localhost:3030"
export IDENA_RPC_KEY="your_idena_rpc_key"

### 4. Run the Web Server


go run main.go

 This starts the backend at http://localhost:3030.

Available routes include:

    /signin – initiates login with Idena

    /callback – handles return from the Idena app

    /whitelist/current – whitelist for the active epoch

    /whitelist/epoch/{epoch} – whitelist for a specific epoch

    /whitelist/check?address=... – checks one address

Example:

```bash
curl -X GET "http://localhost:3030/whitelist/check?address=0xYourAddress"
```

    /merkle_root – (to be implemented)

### 5. Build & Run the Rolling Indexer

`rolling_indexer/main.go` polls an Idena node and writes identity snapshots to an SQLite database.
The default database file is `identities.db` inside the `rolling_indexer` directory.

To build and launch the service:

```bash
cd rolling_indexer
go build -o rolling-indexer main.go

# environment variables override config.json
export RPC_URL="http://localhost:9009"     # node RPC endpoint
export RPC_KEY="your_rpc_key"              # if your node requires an API key
export FETCH_INTERVAL_MINUTES=10            # how often to poll

./rolling-indexer
```

You may alternatively create a `config.json` with the same fields:

```json
{
  "rpc_url": "http://localhost:9009",
  "rpc_key": "your_rpc_key",
  "interval_minutes": 10,
  "db_path": "identities.db"
}
```

Once running, the indexer serves a REST API on `:8080`. Example queries:

```bash
# latest snapshot of all identities
curl http://localhost:8080/identities/latest

# only addresses currently eligible for PoH
curl http://localhost:8080/identities/eligible

# full history for a single address
curl http://localhost:8080/identity/0x1234...

# addresses filtered by state (Human, Verified, etc.)
curl http://localhost:8080/state/Human
```

### 6. Run the Identity Fetcher Agent (optional)

 Use this to fetch identity snapshots for a list of addresses:

cp agents/fetcher_config.example.json agents/config.json
Edit agents/config.json to match your setup
go run agents/identity_fetcher.go agents/config.json

 It reads address_list.txt, contacts your node (or fallback API), and writes identity data to snapshot.json.

### 7. Find Session Start Blocks (optional)

 Use this helper to detect when the Short and Long Idena sessions begin:

cp agents/session_finder_config.example.json agents/session_config.json
go run agents/session_block_finder.go agents/session_config.json

 It prints the block heights of both session start events.

### 8. Export Merkle Root (upcoming)

 A planned endpoint /merkle_root will:

    - Fetch all eligible addresses from the database

    - Construct a deterministic Merkle tree (using SHA256 or similar)

    - Return the Merkle root hash in JSON

 This is designed for:

    - Circles group minting

    - Gnosis Safe or Idena–EVM bridges

    - On-chain eligibility verification

 You can contribute to this feature – see open issues or the Codex roadmap.

### Disclaimer

 This is a hobby codebase provided strictly for experimental, non-commercial, and private use only.
 No guarantees, representations, or warranties of any kind are made — especially regarding functionality, accuracy, availability, or security.
 Usage is strictly at your own risk. No liability is accepted for any direct or indirect damages or losses, to the fullest extent permitted by law.
Brain users preferred 😉
