// Full audit logging, API docs/Swagger, contributor onboarding UI
/*
# Example audit log fetch:
fetch('/admin/auditlog').then(res => res.json()).then(setAuditLog)
*/

import React, { useState, useEffect } from 'react'

export default function MerkleWhitelistAdminDashboard() {
  // --- Audit log state ---
  const [auditLog, setAuditLog] = useState([])
  const [logFilter, setLogFilter] = useState('')
  const [logPage, setLogPage] = useState(1)
  const [logPageSize] = useState(50)
  // --- API docs ---
  const [showApiDocs, setShowApiDocs] = useState(false)
  // --- Contributing/onboarding ---
  const [showOnboarding, setShowOnboarding] = useState(false)

  // --- Fetch audit log (on mount or on demand) ---
  useEffect(() => {
    fetch('/admin/auditlog')
      .then(res => res.json())
      .then(setAuditLog)
  }, [])

  // --- Export audit log as CSV ---
  function exportAuditCSV() {
    const csv =
      'Timestamp,User,Action,Address,Result\n' +
      auditLog.map(row =>
        [row.timestamp, row.user, row.action, row.address, row.result].join(',')
      ).join('\n')
    navigator.clipboard.writeText(csv)
  }

  // --- Filtered/paginated logs ---
  const filteredLog = auditLog.filter(row =>
    !logFilter ||
    row.user?.toLowerCase().includes(logFilter.toLowerCase()) ||
    row.action?.toLowerCase().includes(logFilter.toLowerCase()) ||
    row.address?.toLowerCase().includes(logFilter.toLowerCase())
  )
  const pagedLog = filteredLog.slice((logPage-1)*logPageSize, logPage*logPageSize)

  // --- UI ---
  return (
    <div className="mt-6">
      {/* Sidebar/menu: Tabs for Audit Log, API Docs, Onboarding */}
      <div className="flex gap-4 mb-4">
        <button className="btn btn-xs" onClick={() => setShowApiDocs(false) || setShowOnboarding(false)}>
          Audit Log
        </button>
        <button className="btn btn-xs" onClick={() => setShowApiDocs(true)}>
          API Docs
        </button>
        <button className="btn btn-xs" onClick={() => setShowOnboarding(true)}>
          Developers / Contributing
        </button>
      </div>

      {/* --- AUDIT LOG TAB --- */}
      {!showApiDocs && !showOnboarding && (
        <div>
          <div className="flex gap-2 mb-2">
            <input
              className="border rounded p-1 text-xs"
              placeholder="Filter by user/action/address..."
              value={logFilter}
              onChange={e => setLogFilter(e.target.value)}
            />
            <button className="btn btn-xs" onClick={exportAuditCSV}>
              Export CSV
            </button>
            <span className="ml-2 text-xs text-gray-600">
              {filteredLog.length} log entries
            </span>
          </div>
          <table className="w-full text-xs border mb-2">
            <thead>
              <tr>
                <th>Timestamp</th>
                <th>User</th>
                <th>Action</th>
                <th>Address</th>
                <th>Result</th>
              </tr>
            </thead>
            <tbody>
              {pagedLog.map((row, i) => (
                <tr key={i}>
                  <td>{row.timestamp}</td>
                  <td>{row.user}</td>
                  <td>{row.action}</td>
                  <td>{row.address}</td>
                  <td>{row.result}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {/* Pagination */}
          <div className="text-xs">
            Page {logPage} / {Math.ceil(filteredLog.length/logPageSize)}
            <button className="ml-2 btn btn-xs"
              onClick={() => setLogPage(p => Math.max(1, p-1))}
              disabled={logPage <= 1}>Prev</button>
            <button className="ml-2 btn btn-xs"
              onClick={() => setLogPage(p => p+1)}
              disabled={logPage*logPageSize >= filteredLog.length}>Next</button>
          </div>
        </div>
      )}

      {/* --- API DOCS TAB --- */}
      {showApiDocs && (
        <div>
          <div className="mb-2 font-bold">API Documentation (Swagger)</div>
          <iframe
            src="/swagger"
            className="w-full h-96 border rounded"
            title="Swagger API Docs"
          />
          <div className="text-xs text-gray-600 mt-2">
            <a href="/openapi.json" target="_blank" rel="noopener noreferrer">
              Download OpenAPI Spec (JSON)
            </a>
          </div>
        </div>
      )}

      {/* --- ONBOARDING/CONTRIBUTORS TAB --- */}
      {showOnboarding && (
        <div className="bg-gray-50 rounded p-6">
          <h2 className="font-bold mb-2">Developer & Contributor Guide</h2>
          <ul className="list-disc ml-6 mb-3">
            <li>Clone: <code>git clone https://github.com/ubiubi18/IdenaAuthGo.git</code></li>
            <li>Install deps: <code>npm install</code> or <code>yarn</code></li>
            <li>Run dev: <code>npm run dev</code></li>
            <li>API: Use <code>/swagger</code> for docs, <code>/admin/auditlog</code> for audit, <code>/identity/0x.../history</code> for audit per address</li>
            <li>Get API key: <a href="/settings" className="underline">via Settings</a> (if available)</li>
            <li>Style: Prettier/linting enforced, see <a href="/CONTRIBUTING.md" className="underline">CONTRIBUTING.md</a></li>
            <li>PRs: Welcome, with tests &amp; docs!</li>
            <li>Dependencies: see <a href="/package.json" className="underline">package.json</a></li>
          </ul>
          <div className="mb-2">
            <b>GitHub:</b> <a href="https://github.com/ubiubi18/IdenaAuthGo" target="_blank" rel="noopener noreferrer" className="underline">ubiubi18/IdenaAuthGo</a>
          </div>
          <div className="mb-2 text-xs text-gray-500">
            <span className="mr-3">Build: <img src="https://img.shields.io/github/actions/workflow/status/ubiubi18/IdenaAuthGo/ci.yml" alt="build status" className="inline-block align-middle" /></span>
            <span className="mr-3">License: MIT</span>
            <span>Version: 1.0.0 (dev)</span>
          </div>
        </div>
      )}

      {/* Footer: version/environment */}
      <div className="mt-8 text-xs text-gray-400 text-center">
        Merkle Whitelist Generator v1.0.0 – {process.env.NODE_ENV}
      </div>
    </div>
  )
}
