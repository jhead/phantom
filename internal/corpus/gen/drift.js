#!/usr/bin/env node
//
// Matrix drift check.
//
// Compares the Minecraft versions bedrock-protocol supports against what this
// repo actually tests, and fails if we have fallen behind. This is what keeps
// "N versions" current without anyone having to remember: when Mojang ships a
// new version and bedrock-protocol adds it, the nightly job goes red.
//
// Usage: node drift.js
// Exit 0 = in sync, exit 1 = drift found.

const fs = require('fs')
const path = require('path')

const supported = require('bedrock-protocol/src/options').Versions

const capturedPath = path.join(__dirname, '..', 'data', 'captured.json')
const matrixPath = path.join(__dirname, '..', '..', '..', 'test', 'compat', 'matrix.json')

const captured = JSON.parse(fs.readFileSync(capturedPath, 'utf8'))
const matrix = JSON.parse(fs.readFileSync(matrixPath, 'utf8'))

const capturedVersions = new Set(captured.entries.map(e => e.mc))
const matrixVersions = new Set(matrix.versions.map(v => v.mc))
const supportedVersions = Object.keys(supported)

const missingFromCorpus = supportedVersions.filter(v => !capturedVersions.has(v))

// The T1 matrix is deliberately a SAMPLE, so a version missing from it is only
// notable when it is newer than everything we currently sample.
const newestSampled = Math.max(...[...matrixVersions].map(v => supported[v] || 0))
const newerThanMatrix = supportedVersions.filter(
  v => supported[v] > newestSampled && !matrixVersions.has(v)
)

let drift = false

if (missingFromCorpus.length) {
  drift = true
  console.error('T0 corpus is missing versions bedrock-protocol now supports:')
  for (const v of missingFromCorpus) {
    console.error(`  ${v} (protocol ${supported[v]})`)
  }
  console.error('\nFix: run `make corpus` and commit internal/corpus/data/captured.json.\n')
}

if (newerThanMatrix.length) {
  drift = true
  console.error('T1 matrix does not sample any version this new:')
  for (const v of newerThanMatrix) {
    console.error(`  ${v} (protocol ${supported[v]})`)
  }
  console.error('\nFix: add the newest to test/compat/matrix.json, keeping shards balanced.\n')
}

if (!drift) {
  console.log(`In sync: ${supportedVersions.length} versions supported, ` +
    `${capturedVersions.size} in the T0 corpus, ${matrixVersions.size} sampled by T1.`)
  process.exit(0)
}

process.exit(1)
