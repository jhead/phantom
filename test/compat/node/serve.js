#!/usr/bin/env node
//
// Boots a real bedrock-protocol server and holds it open until killed.
//
// Prints one line of JSON to stdout when listening:  {"ready":true,"port":N}
// The Go harness waits for that line, then points phantom at this port.
//
// Usage: node serve.js --port <port> --version <mcVersion>

const bp = require('bedrock-protocol')

function arg (name, fallback) {
  const i = process.argv.indexOf(`--${name}`)
  return i >= 0 ? process.argv[i + 1] : fallback
}

const port = parseInt(arg('port', '19132'), 10)
const version = arg('version', '1.21.0')

function emit (obj) {
  process.stdout.write(JSON.stringify(obj) + '\n')
}

let server
try {
  server = bp.createServer({
    host: '127.0.0.1',
    port,
    version,
    offline: true,
    motd: { motd: 'Compat Upstream', levelName: 'compat' }
  })
} catch (err) {
  emit({ ready: false, error: err.message })
  process.exit(1)
}

server.on('error', err => {
  emit({ ready: false, error: String(err && err.message || err) })
})

// bedrock-protocol has no single reliable "listening" event across every
// version it supports, so settle briefly and then declare readiness. The Go
// side does not trust this anyway - it polls with a real ping until it gets a
// pong, which is the only readiness signal that actually means anything.
setTimeout(() => emit({ ready: true, port, version }), 1000)

for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => {
    try { server.close() } catch (_) {}
    process.exit(0)
  })
}
