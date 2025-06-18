import React, { useState } from 'react'

// If using Tailwind or shadcn/ui, import here

export default function MerkleWhitelistGenerator() {
  // State variables
  const [merkleRoot, setMerkleRoot] = useState('')
  const [logs, setLogs] = useState([])
  const [loading, setLoading] = useState(false)
  const [address, setAddress] = useState('')
  const [eligibilityResult, setEligibilityResult] = useState(null)

  // Stub: Trigger whitelist generation (replace with real fetch later)
  const handleGenerate = (source) => {
    setLoading(true)
    setLogs([])
    setMerkleRoot('')
    // Simulate log streaming and root generation
    setTimeout(() => {
      setLogs([
        'Fetching identities...',
        'Filtering 500 -> 230 eligible...',
        'Computing Merkle root...',
        'Done!'
      ])
      setMerkleRoot('0x1234abcd...') // stub
      setLoading(false)
    }, 1200)
  }

  // Stub: Address check (replace with fetch to /whitelist/check)
  const handleCheck = () => {
    setEligibilityResult(null)
    setTimeout(() => {
      setEligibilityResult({
        eligible: true,
        status: 'Human',
        stake: '20,500 iDNA',
        proof: ['0xaaa...', '0xbbb...'] // stub
      })
    }, 800)
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
      <div className="bg-gray-50 rounded p-4">
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
        {eligibilityResult && (
          <div className="text-sm mt-2">
            {eligibilityResult.eligible ? (
              <div>
                <span className="text-green-700 font-semibold">Eligible</span> –
                Status: {eligibilityResult.status}, Stake: {eligibilityResult.stake}
                <br />
                <span>Merkle Proof:</span>
                <ul className="font-mono">
                  {eligibilityResult.proof.map((h, i) => (
                    <li key={i}>{h}</li>
                  ))}
                </ul>
              </div>
            ) : (
              <span className="text-red-700">Not eligible</span>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
