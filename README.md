# 🔐 Idena Eligibility Checker – Minimal Go Backend

This project provides a minimal backend in Go to verify if an address corresponds to a valid Idena identity with a stake over 10,000 iDNA.
✅ Current Features

    Implements the “Sign in with Idena” deep link flow

    Verifies signature and stake using either a local Idena node or the public API

    Confirms eligibility (Newbie, Verified, Human with ≥10k iDNA)

🧭 Roadmap

    - Fetch discriminationStakeThreshold from local node

    - Add lightweight local indexer (track identities over last 30 days)

    - Store identity snapshots in JSON format

    - Implement Merkle tree generator for exportable whitelists

    - Add UI field to compare submitted address against live whitelist

    - Fallback to sign-in with idena if no address is entered

    - Export sorted whitelist of eligible IDs (Human/Verified/Newbie above 10k iDNA or other discriminators)

    - Publish verifiable Merkle root for use on other blockchains


## Requirements

* **OS:** Ubuntu 22.04+ (tested), but any Linux/macOS/Windows with Go 1.19+ will work
* **Go:** v1.19 or newer
* **Git**
* **idena-go node** (for private/production usage; fallback is public API)
* **(Optional)**: Domain and SSL for public/prod (required for app.idena.io sign-in!)

---

## Quick Start

### 1. Install dependencies (Ubuntu example)

```bash
sudo apt update
sudo apt install git golang sqlite3
```

### 2. Clone and Build

```bash
git clone https://github.com/ubiubi18/IdenaAuthGo.git
cd IdenaAuthGo
go build -o idenauthgo main.go
```

### 3. Configure your environment

Copy and edit your environment file:

```bash
cp .env.example .env
nano .env
```

**Edit these values as needed:**

```env
# BASE_URL sets the public address where your backend is reachable by browsers and by app.idena.io.
# For a VPS setup with IP only:
BASE_URL="http://YOUR_SERVER_IP:3030"
# For local use with desktop app:
# BASE_URL="http://localhost:3030"
# For production with a domain:
# BASE_URL="https://yourdomain.tld"

# To use app.idena.io sign-in with Callback to website you MUST use HTTPS and a domain!

# IDENA_RPC_KEY is your idena-go node API key.
# To generate a new API key in console:
#   openssl rand -hex 16
# Paste output below and use the same for idena-go startup.
IDENA_RPC_KEY="your-node-api-key"
```
## Running IdenaAuthGo: Local vs. Public

**Personal/local mode (Desktop only):**
- Set `BASE_URL="http://localhost:3030"` in `.env`
- Start backend and idena-go locally
- Open browser on **the same PC** to `http://localhost:3030/signin`
- Sign in with your Desktop Idena app (deep link works)
- No mobile or web support!

**Public/VPS/production mode (for remote/web/mobile):**
- Set `BASE_URL="http://YOUR_IP:3030"` or `BASE_URL="https://yourdomain.tld"`
- Start backend and idena-go on server
- Open port 3030 to the world (firewall/router)
- Anyone can sign in using app.idena.io or the mobile app
- Callbacks and nonces are reachable over the internet

---

### 4. Start your idena-go node

Download the latest release from [https://github.com/idena-network/idena-go/releases](https://github.com/idena-network/idena-go/releases) and start it:

```bash
nohup idena-go --rpcaddr 127.0.0.1 --rpcport 9009 --apikey YOUR_API_KEY --datadir ~/.idena > idena-node.log 2>&1 &
```

* Ensure `YOUR_API_KEY` matches your `.env` IDENA\_RPC\_KEY
* Restart node if you change the key
* The API key is also stored in `~/.idena/api.key` (overrides if present)

---

### 5. Start the backend

```bash
cd ~/IdenaAuthGo
set -a
. ./.env
set +a
go build -o idenauthgo main.go
pkill idenauthgo || true
nohup ./idenauthgo > idenauthgo.log 2>&1 &
tail -f idenauthgo.log
```

* All logs (auth, errors, RPC calls) appear in `idenauthgo.log`

---

## Usage

* Open your browser to `BASE_URL/signin` (e.g., [http://localhost:3030/signin](http://localhost:3030/signin) or [https://yourdomain.tld/signin](https://yourdomain.tld/signin))
* Complete sign-in with Idena Web App or Desktop App
* The result page displays address, status, and stake

You do **NOT** need a domain for local or internal usage. For production and app.idena.io, use HTTPS + a domain.

---

## Advanced Notes

### Firewall / Ports

* Make sure port 3030 is open if accessed from LAN/internet.

### HTTPS (SSL) / Reverse Proxy

* If your backend runs HTTP, but your domain is HTTPS, use a reverse proxy (e.g., nginx) to forward traffic securely.
* Local development works fine with plain HTTP.

### API Key Troubleshooting

* If you see `the provided API key is invalid`, make sure both your `.env` and `idena-go` use **exactly the same key**.
* Always restart idena-go after changing the key.

### Node Not Responding?

* Check if node runs: `ps aux | grep idena-go`
* Check port: `netstat -tuln | grep 9009`
* Test direct call:

  ```bash
  curl -X POST http://127.0.0.1:9009 \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","id":1,"method":"dna_identity","params":["<ADDRESS>"],"key":"<API_KEY>"}'
  ```

### Session / DB

* If you have DB/session issues, delete `sessions.db` and restart backend.

---

## Example .env

```env
BASE_URL="https://yourdomain.tld"
IDENA_RPC_KEY="your-node-api-key"
```
---

## License

MIT License – use, fork, or contribute as you wish.


---

## Community / Help

Questions? Issues? PRs welcome at [https://github.com/ubiubi18/IdenaAuthGo](https://github.com/ubiubi18/IdenaAuthGo)

## Disclaimer
This is a hobby project, built for fun and experimental use by curious minds. Brainy users especially welcome! Please note: it’s strictly for private, non-commercial use. There are no guarantees, promises, or warranties of any kind—about features, correctness, uptime, or security. You use this entirely at your own risk. The creators accept no responsibility or liability for any loss or damage, as far as the law allows.
