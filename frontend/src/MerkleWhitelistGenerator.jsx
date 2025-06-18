import React, { useState, useRef } from 'react'

// If using Tailwind or shadcn/ui, import here

export default function MerkleWhitelistGenerator() {
  // State variables
  const [merkleRoot, setMerkleRoot] = useState('')
  const [epoch, setEpoch] = useState(null)
  const [logs, setLogs] = useState([])
  const [loading, setLoading] = useState(false)
  const [address, setAddress] = useState('')
  const [eligibilityResult, setEligibilityResult] = useState(null)
  const [error, setError] = useState(null)
  const eventSourceRef = useRef(null)

  // Utility to append a log line
  const appendLog = (line) => setLogs((prev) => [...prev, line])

  // Backend URL base (adjust as needed)
  const API_BASE = 'http://localhost:3030'

  // Color map for status badges
  const statusColors = {
    Human: 'bg-green-200 text-green-800',
    Verified: 'bg-blue-200 text-blue-800',
    Newbie: 'bg-yellow-100 text-yellow-800',
    Suspended: 'bg-red-200 text-red-800',
    Zombie: 'bg-gray-200 text-gray-700',
    Killed: 'bg-gray-200 text-gray-700',
    Undefined: 'bg-gray-100 text-gray-500'
  }

  // Format stake (thousands separator)
  function formatStake(stake) {
    if (!stake) return ''
    return (
      Number(stake).toLocaleString('en-US', { maximumFractionDigits: 3 }) +
      ' iDNA'
    )
  }

  // Start Merkle root generation with log streaming
  const handleGenerate = async (source) => {
    setLoading(true)
    setLogs([])
    setMerkleRoot('')
    setEpoch(null)
    setError(null)

    try {
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
      }
      const es = new window.EventSource(`${API_BASE}/logs/stream`)
      eventSourceRef.current = es
      es.onmessage = (event) => {
        const data = event.data
        if (data === '[DONE]') {
          es.close()
          fetchMerkleRoot()
          setLoading(false)
        } else {
          appendLog(data)
        }
      }
      es.onerror = () => {
        es.close()
        appendLog('[Error: log stream failed, fallback to polling logs endpoint or check backend]')
        setLoading(false)
      }

      await fetch(`${API_BASE}/generate_merkle?source=${source}`, { method: 'POST' })
    } catch (err) {
      setError('Failed to start whitelist generation')
      setLoading(false)
    }
  }

  // Fetch Merkle root and epoch after generation
  const fetchMerkleRoot = async () => {
    try {
      const res = await fetch(`${API_BASE}/merkle_root`)
      const data = await res.json()
      setMerkleRoot(data.merkle_root || '')
      setEpoch(data.epoch || null)
      appendLog(`[Merkle Root]: ${data.merkle_root} (Epoch ${data.epoch})`)
    } catch (err) {
      appendLog('[Error: Failed to fetch Merkle root]')
    }
  }

  // Address eligibility and proof check with reason/explanations
  const handleCheck = async () => {
    setEligibilityResult(null)
    setError(null)
    try {
      // Fetch eligibility
      const res = await fetch(`${API_BASE}/whitelist/check?address=${address}`)
      const data = await res.json()
      let reasons = []
      if (Array.isArray(data.reasons)) reasons = data.reasons
      else if (data.reason) reasons = [data.reason]
      // If eligible, fetch proof as well
      if (data.eligible) {
        const proofRes = await fetch(`${API_BASE}/merkle_proof?address=${address}`)
        const proofData = await proofRes.json()
        setEligibilityResult({
          eligible: true,
          status: data.status,
          stake: data.stake,
          reasons: [],
          proof: proofData.proof || []
        })
      } else {
        setEligibilityResult({
          eligible: false,
          status: data.status,
          stake: data.stake,
          reasons
        })
      }
    } catch (err) {
      setError('Eligibility check failed. Check address and try again.')
    }
  }

  // Copy proof as JSON array
  function copyProof(proof) {
    navigator.clipboard.writeText(JSON.stringify(proof))
  }

  return (
    <div className="max-w-xl mx-auto p-6">
      {/* Title */}
      <h1 className="text-2xl font-bold text-center mb-4">
        Idena Eligibility Discriminator – Generate Whitelist Merkle Root
      </h1>

      {/* Mode Buttons */}
      <div className="flex justify-center gap-4 mb-2">
        <button
          className="btn btn-primary"
          disabled={loading}
          onClick={() => handleGenerate('node')}
        >
          From your own node
        </button>
        <button
          className="btn btn-secondary"
          disabled={loading}
          onClick={() => handleGenerate('public')}
        >
          From the public indexer
        </button>
      </div>

      {/* Description */}
      <div className="text-gray-600 text-center mb-4">
        Checks and filters Idena identities by PoP rules (status and stake) to generate a deterministic whitelist for the current epoch. The result is a Merkle root and inclusion proofs for eligibility verification. You can use your own node or fall back to a public indexer.
      </div>

      {/* Live Log Panel */}
      <div className="bg-black text-green-300 font-mono rounded p-2 h-32 overflow-y-auto mb-4">
        {logs.length === 0 ? (
          <span className="opacity-50">Console output will appear here…</span>
        ) : (
          logs.map((line, idx) => <div key={idx}>{line}</div>)
        )}
      </div>

      {/* Merkle Root Display */}
      <div className="mb-4 flex items-center gap-2">
        <input
          className="flex-1 border rounded p-2"
          readOnly
          value={merkleRoot}
          placeholder="Merkle root will appear here…"
        />
        <button
          className="btn btn-outline"
          disabled={!merkleRoot}
          onClick={() => navigator.clipboard.writeText(merkleRoot)}
        >
          Copy
        </button>
      </div>

      {/* Address Checker */}
      <div className="bg-gray-50 rounded p-4 mt-8">
        <h2 className="font-semibold mb-2">Check Address</h2>
        <div className="flex gap-2 mb-2">
          <input
            className="flex-1 border rounded p-2"
            placeholder="0x..."
            value={address}
            onChange={e => setAddress(e.target.value)}
          />
          <button className="btn btn-accent" onClick={handleCheck}>
            Check
          </button>
        </div>
        {/* Eligibility result with detail */}
        {eligibilityResult && (
          <div className="mt-2 border rounded-lg p-4 shadow bg-white">
            {/* Eligibility status */}
            <div className="flex items-center gap-4 mb-2">
              <span
                className={
                  'text-xl font-bold ' +
                  (eligibilityResult.eligible ? 'text-green-700' : 'text-red-700')
                }
              >
                {eligibilityResult.eligible ? 'Eligible' : 'Not eligible'}
              </span>
              {/* Status badge */}
              <span
                className={
                  'inline-block px-3 py-1 rounded-full text-xs font-semibold ' +
                  (statusColors[eligibilityResult.status] || statusColors.Undefined)
                }
              >
                {eligibilityResult.status || 'Unknown'}
              </span>
            </div>
            {/* Stake */}
            <div className="mb-2">
              <span className="font-medium">Stake:&nbsp;</span>
              <span className="font-mono font-semibold">
                {formatStake(eligibilityResult.stake)}
              </span>
            </div>
            {/* Not eligible: list all exclusion reasons */}
            {!eligibilityResult.eligible && (
              <div>
                <div className="font-semibold text-red-700">Exclusion reason(s):</div>
                <ul className="list-disc pl-6 text-sm">
                  {eligibilityResult.reasons?.length ? (
                    eligibilityResult.reasons.map((r, i) => <li key={i}>{r}</li>)
                  ) : (
                    <li>No reason given</li>
                  )}
                </ul>
              </div>
            )}
            {/* Eligible: Merkle proof display + copy button */}
            {eligibilityResult.eligible && (
              <div>
                <div className="font-semibold mt-2">Merkle Proof:</div>
                <pre className="bg-gray-100 rounded p-2 text-xs max-h-32 overflow-x-auto mb-1">
                  {JSON.stringify(eligibilityResult.proof, null, 2)}
                </pre>
                <button
                  className="btn btn-outline btn-xs"
                  onClick={() => copyProof(eligibilityResult.proof)}
                >
                  Copy Merkle Proof
                </button>
              </div>
            )}
          </div>
        )}
        {error && <div className="text-red-600 mt-2">{error}</div>}
      </div>
    </div>
  )
}
