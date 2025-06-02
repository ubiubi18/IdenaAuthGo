# IdenaAuthGo

⚠️ DISCLAIMER: WORK IN PROGRESS – NOT PRODUCTION READY – RAPIDLY DEVELOPED WITH GPT-4.1 (OPENAI) – USE WITH CAUTION – FOR DEVELOPMENT & TESTING ONLY ⚠️

A minimal Go backend for "Sign in with Idena" and Proof-of-Personhood verification.

✅ Current Functionality

This backend verifies whether a given address corresponds to a valid, Idena identity with a stake over 10,000 iDNA. It provides simple endpoints for integration in web apps, DApps, and automation tools.

Valid identities trigger a signed response.

Invalid or non-eligible identities (e.g., random addresses or lower-tier statuses) result so far in a fallback to an about:blank popup.


🛠️ Planned Roadmap

Implementation of a reproducible and verifiable Merkle root based on Idena identity snapshots - Integrate an adapted version of Identity Bridge 
https://docs.idena.io/docs/developer/identity-bridge
Snapshot the validation state of all identities with ≥10,000 iDNA stake (or alternatively, above the discriminationStakeThreshold once implemented).

Hash the resulting list into a Merkle tree.

Export the Merkle root for cross-chain whitelisting, e.g., in GnosisChain smart contracts or Circles Group Currency systems.

---

## Features

* **Idena Sign-In:** Full protocol integration, compatible with Idena Web App and Desktop App.
* **Eligibility Check:** Accepts only Human/Verified/Newbie identities with configurable minimum stake (default: 10,000 iDNA).
* **REST API endpoints** for easy integration.
* **Fallback to public Idena indexer** if your node fails.
* **Detailed logging** and transparent error messages for easy debugging.
* **MIT licensed, minimal, easy to fork.**

---

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

## Security

* Never commit your real `.env` file. Only `.env.example` is for the repo.
* Use strong random API keys.
* For public deployment, always use HTTPS and a real VPS/server.

---

## License

MIT License – use, fork, or contribute as you wish.

---

## Community / Help

Questions? Issues? PRs welcome at [https://github.com/ubiubi18/IdenaAuthGo](https://github.com/ubiubi18/IdenaAuthGo)

## Disclaimer
This is a hobby vibe code project, provided strictly for experimental, non-commercial, and private use only. No guarantees, representations, or warranties of any kind are made—especially regarding functionality, accuracy, availability, or security. Usage is strictly at your own risk. No liability is accepted for any direct or indirect damages or losses, to the fullest extent permitted by law.
