import React from 'react'
import { createRoot } from 'react-dom/client'
import MerkleWhitelistGenerator from './MerkleWhitelistGenerator'
import MerkleWhitelistAdminTools from './MerkleWhitelistAdminTools'

const root = createRoot(document.getElementById('root'))
root.render(
  <>
    <MerkleWhitelistGenerator />
    <MerkleWhitelistAdminTools />
  </>
)
