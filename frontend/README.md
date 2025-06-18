# Frontend

This directory contains a React-based interface for generating the Idena whitelist
Merkle root and checking an address against it.

The UI now connects to the Go backend (`http://localhost:3030` by default) and
streams log output from `/logs/stream` while the whitelist is being built. Make
sure the main server and rolling indexer are running:

```bash
go run main.go
# In another terminal
cd rolling_indexer && go build -o rolling-indexer main.go
export RPC_URL="http://localhost:9009"
./rolling-indexer
```

To develop the frontend, install dependencies and use a bundler of your choice
(for example `vite`). The entry point is `index.html` which loads
`src/index.jsx`.
