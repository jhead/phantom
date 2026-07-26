#!/usr/bin/env node
//
// Captures real RakNet Unconnected Pong (0x1c) bytes from a bedrock-protocol
// server, one entry per supported Minecraft version, into ../data/captured.json.
//
// Run via `make corpus` from the repo root. This is a DEV-TIME task: CI never
// runs it, it only replays the committed bytes. Regenerating and diffing the
// result is how we detect that a new Minecraft version changed the pong shape.
//
// Usage: node capture.js [--out <path>] [--only <version>[,<version>...]]

const dgram = require('dgram')
const fs = require('fs')
const path = require('path')
const bp = require('bedrock-protocol')

const MAGIC = Buffer.from('00ffff00fefefefefdfdfdfd12345678', 'hex')

// Fixed, arbitrary values so captures are reproducible and so Go tests can
// assert that the server echoed OUR ping time back (RakNet requires it).
const PING_TIME = '0011223344556677'
const CLIENT_GUID = '8877665544332211'

const BASE_PORT = 19300
const BOOT_MS = 1500
const PING_TIMEOUT_MS = 5000
const PING_RETRIES = 3

function versions () {
  // bedrock-protocol keeps its version->protocol table here.
  return require('bedrock-protocol/src/options').Versions
}

function buildPing () {
  return Buffer.concat([
    Buffer.from([0x01]),
    Buffer.from(PING_TIME, 'hex'),
    MAGIC,
    Buffer.from(CLIENT_GUID, 'hex')
  ])
}

// Sends an Unconnected Ping and returns the raw pong datagram.
function pingOnce (port) {
  return new Promise((resolve, reject) => {
    const sock = dgram.createSocket('udp4')
    const timer = setTimeout(() => {
      sock.close()
      reject(new Error('ping timeout'))
    }, PING_TIMEOUT_MS)

    sock.on('error', err => {
      clearTimeout(timer)
      sock.close()
      reject(err)
    })

    sock.on('message', msg => {
      clearTimeout(timer)
      sock.close()
      resolve(msg)
    })

    sock.send(buildPing(), port, '127.0.0.1', err => {
      if (err) {
        clearTimeout(timer)
        sock.close()
        reject(err)
      }
    })
  })
}

async function captureVersion (mcVersion, port) {
  let server
  try {
    server = bp.createServer({
      host: '127.0.0.1',
      port,
      version: mcVersion,
      offline: true
    })
  } catch (err) {
    throw new Error(`createServer failed: ${err.message}`)
  }

  try {
    await new Promise(r => setTimeout(r, BOOT_MS))

    let pong
    let lastErr
    for (let attempt = 0; attempt < PING_RETRIES; attempt++) {
      try {
        pong = await pingOnce(port)
        break
      } catch (err) {
        lastErr = err
      }
    }
    if (!pong) throw lastErr

    if (pong[0] !== 0x1c) {
      throw new Error(`expected pong id 0x1c, got 0x${pong[0].toString(16)}`)
    }
    if (pong.length < 35) {
      throw new Error(`pong too short: ${pong.length} bytes`)
    }

    const strLen = pong.readUInt16BE(33)
    const motd = pong.slice(35, 35 + strLen).toString('utf8')

    return {
      mc: mcVersion,
      protocol: versions()[mcVersion],
      pongHex: pong.toString('hex'),
      // Echoed ping time, as the server sent it back. Should equal PING_TIME.
      echoedPingTime: pong.slice(1, 9).toString('hex'),
      serverGUID: pong.slice(9, 17).toString('hex'),
      motd,
      // Note the trailing ';' means the final split element is empty; the real
      // field count is this array minus that empty tail.
      fields: motd.split(';')
    }
  } finally {
    try { server.close() } catch (_) { /* best effort */ }
  }
}

async function main () {
  const args = process.argv.slice(2)
  const outIdx = args.indexOf('--out')
  const outPath = outIdx >= 0
    ? args[outIdx + 1]
    : path.join(__dirname, '..', 'data', 'captured.json')

  const onlyIdx = args.indexOf('--only')
  const only = onlyIdx >= 0 ? args[onlyIdx + 1].split(',') : null

  const table = versions()
  let list = Object.keys(table)
  if (only) list = list.filter(v => only.includes(v))

  // Ascending by protocol number keeps the committed file stable and readable.
  list.sort((a, b) => table[a] - table[b])

  const entries = []
  const failures = []

  for (let i = 0; i < list.length; i++) {
    const mcVersion = list[i]
    const port = BASE_PORT + (i % 150)
    process.stderr.write(`[${i + 1}/${list.length}] ${mcVersion} (proto ${table[mcVersion]}) ... `)
    try {
      const entry = await captureVersion(mcVersion, port)
      entries.push(entry)
      process.stderr.write(`ok, ${entry.fields.length - 1} fields\n`)
    } catch (err) {
      failures.push({ mc: mcVersion, error: err.message })
      process.stderr.write(`FAILED: ${err.message}\n`)
    }
  }

  const pkg = require('./package.json')
  const doc = {
    _comment: [
      'GENERATED FILE - do not edit by hand. Regenerate with `make corpus`.',
      'Real RakNet Unconnected Pong bytes captured from a bedrock-protocol server,',
      'one entry per supported Minecraft version.',
      '',
      'CAVEAT: these are bedrock-protocol advertisements, not Mojang BDS ones.',
      'The MOTD field COUNT is constant across every version here (bedrock-protocol',
      'uses one serializer); only the protocol number and version string vary.',
      'Real per-version shape diversity lives in synthetic.json.'
    ],
    generatedBy: `bedrock-protocol@${pkg.dependencies['bedrock-protocol']}`,
    pingTime: PING_TIME,
    clientGUID: CLIENT_GUID,
    entries
  }

  fs.mkdirSync(path.dirname(outPath), { recursive: true })
  fs.writeFileSync(outPath, JSON.stringify(doc, null, 2) + '\n')

  process.stderr.write(`\nwrote ${entries.length} entries to ${outPath}\n`)
  if (failures.length) {
    process.stderr.write(`${failures.length} failures:\n`)
    for (const f of failures) process.stderr.write(`  ${f.mc}: ${f.error}\n`)
    process.exit(1)
  }
}

main().catch(err => {
  console.error(err)
  process.exit(1)
})
