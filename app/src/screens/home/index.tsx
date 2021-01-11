import React from 'react'
import { SafeAreaView } from 'react-native'
import { EventSubscription, Navigation, Options } from 'react-native-navigation'
import { Styles } from '../../styles'
import type { ServerEntry } from '../../services/Servers'
import Servers from '../../services/Servers'
import ServerList from './ServerList'

interface HomeProps { }
interface HomeState {
  servers: ServerEntry[]
}

class HomeScreen extends React.Component<HomeProps, HomeState> {

  static options: Options = {
    topBar: {
      title: {},
      leftButtons: [
        {
          id: 'id-left',
          icon: {
            system: 'arrow.clockwise.circle'
          }
        }
      ],
      rightButtons: [
        {
          id: 'id-right',
          icon: {
            system: 'plus.circle'
          },
          systemItem: 'add'
        },
      ]
    }
  } as Options

  private navigationEventListener: EventSubscription

  constructor(props: HomeProps) {
    super(props)

    this.state = {
      servers: []
    }
  } 

  private refreshServers() {
    Servers.list()
      .then(servers => {
        console.log(servers)
        this.setState({ servers })
      })
      .catch(console.log)
  }

  componentDidMount() {
    this.navigationEventListener = Navigation.events().bindComponent(this)
    this.refreshServers()
  }

  componentWillUnmount() {
    this.navigationEventListener?.remove()
  }

  render() {
    const { servers } = this.state
  
    return (
      <>
        <SafeAreaView style={ Styles.rootView }>
          <ServerList servers={ servers } />
        </SafeAreaView>
      </>
    )
  }

  async navigationButtonPressed({ buttonId }) {
    console.log(buttonId)

    // todo
    if (buttonId === 'id-left') {
      this.refreshServers()
    } else if (buttonId === 'id-right') {
      await Servers.create({
        id: 'test-' + (this.state.servers.length + 1),
        name: 'Test Server',
        prefs: {
          remoteServer: 'lax.mcbr.cubed.host:19132',
          bindPort: 0,
          bindAddress: '0.0.0.0',
          ipv6: true,
          idleTimeout: 0
        }
      })

      this.refreshServers()
    }
  }

}

export default HomeScreen