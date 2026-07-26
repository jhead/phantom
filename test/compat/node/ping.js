#!/usr/bin/env node
//
// Pings a host with bedrock-protocol's OWN advertisement parser and prints the
// parsed result as JSON.
//
// This is the point of involving Node at all: phantom's rewritten pong gets
// judged by a genuine third-party client parser rather than by phantom's own
// (buggy) one, so a malformed rewrite fails loudly instead of round-tripping
// through the same mistake twice.
//
// Usage: node ping.js --host 127.0.0.1 --port <port>
// Output: {"ok":true,"advertisement":{...}} or {"ok":false,"error":"..."}

const bp = require('bedrock-protocol')

function arg (name, fallback) {
  const i = process.argv.indexOf(`--${name}`)
  return i >= 0 ? process.argv[i + 1] : fallback
}

const host = arg('host', '127.0.0.1')
const port = parseInt(arg('port', '19132'), 10)
const timeoutMs = parseInt(arg('timeout', '5000'), 10)

const timer = setTimeout(() => {
  process.stdout.write(JSON.stringify({ ok: false, error: 'ping timed out' }) + '\n')
  process.exit(1)
}, timeoutMs)

bp.ping({ host, port })
  .then(res => {
    clearTimeout(timer)
    process.stdout.write(JSON.stringify({ ok: true, advertisement: res }) + '\n')
    process.exit(0)
  })
  .catch(err => {
    clearTimeout(timer)
    process.stdout.write(JSON.stringify({
      ok: false,
      error: String(err && err.message || err)
    }) + '\n')
    process.exit(1)
  })
