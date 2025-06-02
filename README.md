# IdenaAuthGo

A minimal Go backend for "Sign in with Idena" and advanced Proof-of-Humanity checks.

---

## Features

- **Idena Sign-In:** Integrates with Idena Web App and Desktop.
- **Eligibility Check:** Only allows Human/Verified/Newbie identities with a minimum stake (configurable).
- **REST API endpoints:** For easy frontend or DApp integration.
- **Fallback to public Idena indexer:** If your own node is unavailable.
- **Detailed logging:** All requests, errors, and node responses.
- **MIT Licensed & minimal.**

---

## Requirements

- **OS:** Ubuntu 22.04+ (tested) or any system with Go
- **Go:** 1.19 or newer
- **Git**
- **idena-go:** Download and run your own node ([releases](https://github.com/idena-network/idena-go/releases))
- **Optional:** A public domain & SSL for production

---

## Installation

### 1. Clone and build
```bash
git clone https://github.com/ubiubi18/IdenaAuthGo.git
cd IdenaAuthGo
sudo apt update
sudo apt install git golang sqlite3
go build -o idenauthgo main.go
```

### 2. Configure your environment

Copy and edit your environment file:
```bash
cp .env.example .env
nano .env
```
**Set the following:**
- `BASE_URL` — How the backend is reached by browsers (e.g. `http://localhost:3030`, `http://YOUR_IP:3030`, or `https://yourdomain.tld`)
- `IDENA_RPC_KEY` — The API key for your idena-go node (find or set it in `~/.idena/api.key` or pass `--apikey` when running node)

### 3. Start your Idena node

Download from [idena-go releases](https://github.com/idena-network/idena-go/releases), then:
```bash
nohup idena-go --rpcaddr 127.0.0.1 --rpcport 9009 --apikey YOUR_API_KEY --datadir ~/.idena > idena-node.log 2>&1 &
```

### 4. Start the backend

```bash
set -a
. ./.env
set +a
go build -o idenauthgo main.go
pkill idenauthgo || true
nohup ./idenauthgo > idenauthgo.log 2>&1 &
tail -f idenauthgo.log
```

---

## Usage

- Open your browser to: `${BASE_URL}/signin`
- The backend will redirect to the Idena App for authentication.
- After authentication, you’ll see eligibility and status info.

---

## Troubleshooting

### API Key problems
- If you see "the provided API key is invalid", make sure the key in `.env` matches the node’s key exactly.
- Change key? Restart both the node and backend!

### Node/Port not reachable
- Is the node running?  
  `ps aux | grep idena-go`
- Is the port open?  
  `netstat -tuln | grep 9009`
- Test with curl:  
  ```bash
  curl -X POST http://127.0.0.1:9009     -H "Content-Type: application/json"     -d '{"jsonrpc":"2.0","id":1,"method":"dna_identity","params":["0xYourAddress"],"key":"YourApiKey"}'
  ```

### HTTPS / HTTP conflicts
- If running a public website, use a reverse proxy (Nginx etc) for HTTPS.
- For local/LAN development, plain HTTP is fine.

### Sessions / DB issues
- If you get stuck, delete `sessions.db` and restart the backend.

---

## Security

- Never commit your `.env` file (use only `.env.example` for sharing).
- Always use a strong API key for production.
- Don’t expose your node to the public unless needed.

---

## Example .env

```env
BASE_URL="https://your-website.io"
IDENA_RPC_KEY="your-node-api-key"
```

---

## License

MIT License — use, fork, or contribute!

**PRs welcome!**

