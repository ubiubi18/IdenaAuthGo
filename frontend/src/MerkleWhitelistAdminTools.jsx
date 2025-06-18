// Custom eligibility scripting, batch tools, extension/webhook hooks
import React, { useState } from 'react'

export default function MerkleWhitelistAdminTools() {
  // --- Custom eligibility scripting ---
  const [showScriptPanel, setShowScriptPanel] = useState(false)
  const [script, setScript] = useState(`function isEligible(identity) {
  if (identity.state === 'Zombie') return false;
  if (identity.state === 'Human') return identity.stake >= 20000;
  return identity.stake >= 10000;
}`)
  const [scriptError, setScriptError] = useState('')
  const [customWhitelist, setCustomWhitelist] = useState([])

  // --- Batch checking ---
  const [batchInput, setBatchInput] = useState('')
  const [batchResults, setBatchResults] = useState([])
  const [processing, setProcessing] = useState(false)

  // --- Integration hooks ---
  const [webhookUrl, setWebhookUrl] = useState('')
  const [webhookStatus, setWebhookStatus] = useState('')

  // --- Apply custom rule to local whitelist ---
  function handleApplyScript(localIdentities) {
    setScriptError('')
    try {
      // eslint-disable-next-line no-new-func
      const fn = new Function('identity', script + '\nreturn isEligible(identity);')
      const filtered = localIdentities.filter(fn)
      setCustomWhitelist(filtered)
    } catch (err) {
      setScriptError('Script error: ' + err.message)
    }
  }

  // --- Batch address checking (demo) ---
  async function handleBatchCheck() {
    setProcessing(true)
    const addresses = batchInput
      .split(/[\s,;]+/)
      .map((x) => x.trim())
      .filter(Boolean)
    // TODO: replace with real API calls
    const results = addresses.map((addr) => ({
      address: addr,
      eligible: Math.random() > 0.5,
      status: 'Human',
      stake: Math.round(10000 + Math.random() * 30000),
      reasons: ['(stub result: replace with real API call)'],
    }))
    setBatchResults(results)
    setProcessing(false)
  }

  // --- Webhook integration: send POST with results ---
  async function sendWebhook() {
    try {
      const payload = {
        timestamp: new Date().toISOString(),
        results: batchResults,
      }
      const res = await fetch(webhookUrl, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      if (res.ok) setWebhookStatus('Sent!')
      else setWebhookStatus('Error: ' + res.status)
    } catch (err) {
      setWebhookStatus('Error: ' + err.message)
    }
  }

  // --- UI ---
  return (
    <div className="my-6">
      {/* Custom eligibility scripting */}
      <div className="mb-4">
        <button className="btn btn-xs" onClick={() => setShowScriptPanel((s) => !s)}>
          Custom eligibility logic
        </button>
        {showScriptPanel && (
          <div className="bg-gray-100 p-3 mt-2 rounded shadow">
            <div className="mb-2 font-mono text-xs">
              Paste/edit JS logic. Only run in browser. No server execution.
              <br />
              <textarea
                className="w-full font-mono border rounded p-2"
                rows={8}
                value={script}
                onChange={(e) => setScript(e.target.value)}
              />
            </div>
            {scriptError && <div className="text-red-600">{scriptError}</div>}
            <button className="btn btn-xs" onClick={() => handleApplyScript([])}>
              Apply to Whitelist (in browser)
            </button>
            <div className="mt-2 text-xs text-gray-500">
              Results: {customWhitelist.length} addresses (download/export as needed)
            </div>
          </div>
        )}
      </div>

      {/* Batch checking */}
      <div className="mb-4">
        <div className="font-semibold mb-1">Batch Address Checker</div>
        <textarea
          className="w-full border rounded p-2 mb-2"
          placeholder="Paste addresses, one per line or CSV"
          rows={4}
          value={batchInput}
          onChange={(e) => setBatchInput(e.target.value)}
        />
        <button className="btn btn-xs" onClick={handleBatchCheck} disabled={processing}>
          Check
        </button>
        {batchResults.length > 0 && (
          <div className="mt-2">
            <table className="w-full text-xs border">
              <thead>
                <tr>
                  <th>Address</th>
                  <th>Eligible</th>
                  <th>Status</th>
                  <th>Stake</th>
                  <th>Reasons</th>
                </tr>
              </thead>
              <tbody>
                {batchResults.map((row) => (
                  <tr key={row.address} className={row.eligible ? 'bg-green-50' : 'bg-red-50'}>
                    <td>{row.address}</td>
                    <td>{row.eligible ? '✔' : '✖'}</td>
                    <td>{row.status}</td>
                    <td>{row.stake}</td>
                    <td>
                      <ul>
                        {(row.reasons || []).map((r, i) => (
                          <li key={i}>{r}</li>
                        ))}
                      </ul>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <button
              className="btn btn-xs mt-2"
              onClick={() => {
                const csv =
                  'Address,Eligible,Status,Stake,Reasons\n' +
                  batchResults
                    .map((row) =>
                      [
                        row.address,
                        row.eligible ? 'yes' : 'no',
                        row.status,
                        row.stake,
                        (row.reasons || []).join('; '),
                      ].join(',')
                    )
                    .join('\n')
                navigator.clipboard.writeText(csv)
              }}
            >
              Copy CSV
            </button>
          </div>
        )}
      </div>

      {/* Webhook/integration */}
      <div className="mb-4">
        <div className="font-semibold">Webhook / Integration</div>
        <input
          className="border rounded p-2 w-64 mr-2"
          value={webhookUrl}
          onChange={(e) => setWebhookUrl(e.target.value)}
          placeholder="https://your.webhook/api"
        />
        <button className="btn btn-xs" onClick={sendWebhook}>
          Send results
        </button>
        {webhookStatus && <span className="ml-2 text-xs">{webhookStatus}</span>}
        <div className="text-xs text-gray-500 mt-1">
          Payload:
          <pre>
            {JSON.stringify(
              {
                timestamp: new Date().toISOString(),
                results: batchResults.slice(0, 1),
              },
              null,
              2
            )}
          </pre>
        </div>
      </div>
    </div>
  )
}
