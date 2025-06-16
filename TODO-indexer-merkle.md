# Indexer and Merkle Tree Roadmap

This document outlines upcoming work to refactor the snapshot/whitelist logic
into a minimal indexer and to incorporate Merkle tree proofs for whitelist
entries.

## Current Snapshot Mechanism

* `agents/identity_fetcher.go` produces `data/snapshot.json` by polling a list
  of addresses. The backend reads this file in `whitelistCheckHandler` when
  verifying eligibility.
* `exportWhitelist()` in `main.go` writes `data/whitelist_epoch_<n>.json` with
  the current Merkle root and eligible addresses. This happens during cleanup
  and after each authentication.

## 1. Snapshot Generation via Minimal Indexer

* **Goal**: Produce deterministic, reproducible snapshots of all identities for
each epoch.
* **Approach**
  - Build a lightweight Go service that queries the node (or public API as
    fallback) on a fixed schedule.
  - Store identity state and stake in an SQLite database similar to the current
    `rolling_indexer` but focused on the latest epoch only.
  - Provide an endpoint or CLI command to export the whitelist as a sorted list
    of addresses (lowercase) with their state and stake.
  - Use the same eligibility rules as the backend: `state` ∈ {Human, Verified,
    Newbie} **and** `stake` ≥ discrimination stake threshold.
  - Ensure results are saved with timestamps to reproduce snapshots later.

## 2. Merkle Tree Construction

* **Whitelist Export**
  - Sort eligible addresses alphabetically and build a Merkle tree using
    SHA‑256 hashes of lowercase addresses.
  - Persist the tree (or at least the root and address list) so the same input
    always yields the same root.
* **Proof Generation**
  - For a given address, compute the Merkle proof (list of sibling hashes and
    their left/right position).
  - Provide HTTP endpoints such as `/merkle_root` and `/merkle_proof?address=` to
    fetch the root and proof.
  - Include the epoch number in responses so clients know which snapshot was
    used.

## 3. Backend/Frontend Usage

* **Backend**
  - On `/whitelist/check`, in addition to eligibility, optionally return the
    Merkle proof for that address. This allows on-chain contracts to verify
    eligibility without trusting the server.
* **Frontend**
  - Fetch `/merkle_root` once and cache it.
  - After checking eligibility, offer the user a button to copy the Merkle proof
    or submit it to a contract.

## Next Steps

1. Implement the minimal indexer service with a command-line interface.
2. Add database migration scripts and tests for deterministic snapshots.
3. Integrate Merkle root and proof endpoints in the main API.
4. Update the frontend to display loading states and detailed error messages for
   eligibility checks.
5. Expose an endpoint to retrieve a Merkle proof for any address and document
   how to verify it in smart contracts or other clients.
