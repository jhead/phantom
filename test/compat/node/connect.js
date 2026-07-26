#!/usr/bin/env node
//
// Drives a real bedrock-protocol client through phantom and reports how far the
// session got. Success is defined as reaching 'join', which means the RakNet
// handshake (OpenConnectionRequest 1/2, MTU negotiation) AND the login sequence
// both completed - the entire opaque path phantom relays without parsing, and
// all of which break if it mangles, truncates or reorders datagrams.
//
// Deliberately NOT waiting for 'spawn': a client only spawns once the server
// sends start_game and chunk data, which bedrock-protocol's createServer does
// not do on its own. Implementing a mini game server would test game-layer
// semantics that phantom is explicitly opaque to (see DESIGN.md non-goals),
// while adding a large, version-fragile surface that could not distinguish a
// phantom bug from our own incomplete server.
//
// Usage: node connect.js --host 127.0.0.1 --port <port> --version <mcVersion>
// Output: {"ok":true,"joined":true} or {"ok":false,"error":"...","stage":"..."}

const bp = require('bedrock-protocol')

function arg (name, fallback) {
  const i = process.argv.indexOf(`--${name}`)
  return i >= 0 ? process.argv[i + 1] : fallback
}

const host = arg('host', '127.0.0.1')
const port = parseInt(arg('port', '19132'), 10)
const version = arg('version', '1.21.0')
const timeoutMs = parseInt(arg('timeout', '30000'), 10)

let stage = 'connecting'
let done = false

function finish (obj, code) {
  if (done) return
  done = true
  process.stdout.write(JSON.stringify(obj) + '\n')
  process.exit(code)
}

const timer = setTimeout(
  () => finish({ ok: false, error: 'timed out', stage }, 1),
  timeoutMs
)

let client
try {
  client = bp.createClient({
    host,
    port,
    version,
    offline: true,
    username: 'CompatBot',
    skipPing: true
  })
} catch (err) {
  clearTimeout(timer)
  finish({ ok: false, error: err.message, stage: 'createClient' }, 1)
}

// Track progress so a timeout reports HOW FAR the session got - "timed out at
// join" and "timed out at connect" point at very different bugs.
for (const ev of ['connect', 'login', 'status']) {
  client.on(ev, () => { stage = ev })
}

client.on('join', () => {
  stage = 'join'
  clearTimeout(timer)
  try { client.close() } catch (_) {}
  finish({ ok: true, joined: true, stage, version }, 0)
})

client.on('error', err => {
  clearTimeout(timer)
  finish({ ok: false, error: String(err && err.message || err), stage }, 1)
})

client.on('kick', reason => {
  clearTimeout(timer)
  finish({ ok: false, error: 'kicked: ' + JSON.stringify(reason), stage }, 1)
})
