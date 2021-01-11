import type Service from './Service'
import { apiHost } from '../env'

const Endpoints = {
  BASE: '/api',
  SERVERS: 'servers',
  SERVERS_START: 'start',
  SERVERS_STOP: 'stop'
}

const url = (...endpoints: string[]) =>
  `http://${apiHost}` + [Endpoints.BASE, ...endpoints].join('/')

export class ServerEntry {
  constructor(
    readonly id: string,
    readonly name: string,
    readonly prefs: ProxyPrefs
  ) { }
}

export class ProxyPrefs {
  constructor(
    readonly remoteServer: string,
    readonly bindAddress?: string,
    readonly bindPort?: number,
    readonly idleTimeout?: number,
    readonly ipv6?: boolean
  ) { }
}

class Servers implements Service<ServerEntry, string> {

  async list(): Promise<ServerEntry[]> {
    const serverMap: Map<string, ServerEntry> = await fetch(url(Endpoints.SERVERS))
      .then(res => res.json())

      return Object.entries(serverMap).map(([ id, server ]) => {
        server.id = id
        return server
      })
  }

  get(id: string): Promise<ServerEntry> {
    throw new Error('Method not implemented.')
  }

  async create(item: ServerEntry): Promise<void> {
    await fetch(url(Endpoints.SERVERS, item.id), {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(item)
    })
  }

  delete(id: string): Promise<void> {
    throw new Error('Method not implemented.')
  }

  update(id: string, item: ServerEntry): Promise<void> {
    throw new Error('Method not implemented.')
  }

  async start(id: string): Promise<void> {
    await fetch(url(Endpoints.SERVERS, id, Endpoints.SERVERS_START), {
      method: 'PUT'
    })
  }

  async stop(id: string): Promise<void> {
    await fetch(url(Endpoints.SERVERS, id, Endpoints.SERVERS_STOP), {
      method: 'PUT'
    })
  }

}

export default new Servers()
