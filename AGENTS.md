Absolutely! Here’s a proposed `AGENTS.md` for your project—modeled after common AI/Codex agent formats. It assumes your project will use agent-style automation, either now or in the roadmap.

---

# AGENTS.md

This file documents autonomous and semi-autonomous agents (“bots”) used or planned for this repository.

---

## Table of Contents

* [Overview](#overview)
* [Agent Roles](#agent-roles)
* [Configuration](#configuration)
* [Security](#security)
* [Contributing New Agents](#contributing-new-agents)
* [Roadmap](#roadmap)

---

## Overview

Agents in this project are designed to automate repetitive, verifiable, or trustless backend tasks—making the “Sign in with Idena” workflow more robust and decentralized. Each agent operates as a small service, script, or background job with a focused responsibility.

---

## Agent Roles

| Agent Name          | Description                                                                        | Status  |
| ------------------- | ---------------------------------------------------------------------------------- | ------- |
| `identity-fetcher`  | Fetches current Idena identity data from local node and maintains a daily snapshot | Planned |
| `threshold-updater` | Queries discriminationStakeThreshold from node and updates eligibility logic       | Planned |
| `indexer`           | Tracks all identity addresses observed in recent blocks (last 30 days)             | Planned |
| `merkle-builder`    | Builds and exports verifiable Merkle roots from eligible identities                | Planned |
| `api-fallback`      | Switches to public API if local node is unavailable                                | Active  |
| `whitelist-checker` | Provides on-demand address eligibility and whitelist comparison                    | Planned |
| `webhook-agent`     | Notifies other services (Discord, Telegram, etc) on new eligibility events         | Planned |

---

## Configuration

Agents are configured via environment variables and JSON config files:

* `.env` (see `README.md`)
* `config/agents.json` (example structure):

  ```json
  {
    "identity-fetcher": { "interval_minutes": 60 },
    "indexer": { "retention_days": 30 },
    "api-fallback": { "enabled": true }
  }
  ```

---

## Security

* Agents interacting with the node require an API key. **Never commit real API keys to version control.**
* If running webhook integrations, use dedicated bot tokens/secrets and restrict their permissions.

---

## Contributing New Agents

1. Propose your agent’s function via an issue or PR.
2. Follow the [CONTRIBUTING.md](CONTRIBUTING.md) guidelines.
3. Keep agents modular—each agent should do one thing well.

---

## Roadmap

* [ ] Modularize agent runners for parallel execution
* [ ] Add Prometheus/Grafana metrics for all agent jobs
* [ ] Support for external verification (e.g., onchain proof publication)

---

> **Note:** This file is evolving as we automate more project workflows. Contributions and suggestions welcome!
This file is evolving as we automate more project workflows. Contributions and suggestions welcome!
