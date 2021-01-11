import React from 'react'
import {
  FlatList,
  View,
  Text,
  Switch,
  Button
} from 'react-native'
import type { ListRenderItemInfo } from 'react-native'
import { Styles } from '../../styles'
import Servers, { ServerEntry } from '../../services/Servers'
declare interface ServerListProps {
  servers: ServerEntry[]
}

const ServerList = ({ servers }: ServerListProps) => {
  const Item = ({ server }) => (
    <View style={Styles.serverItem}>
      <View style={Styles.serverItemTitle}>
        <Text style={Styles.serverItemName}>{ server.name }</Text>
        <Text style={Styles.serverItemDesc}>{ server.prefs.remoteServer }</Text>
      </View>
      <View style={Styles.serverItemStatusContainer}>
        <Text style={Styles.serverItemStatus}>O</Text>
      </View>
      <View>
        <Button
          title='Start'
          onPress={ () => Servers.start(server.id) } />
      </View>
    </View>
  )

  const renderItem = ({ item }: ListRenderItemInfo<ServerEntry>) => (
    <Item
      key={ item.id }
      server={ item } />
  )

  return (
    <View style={Styles.serverList}>
      <FlatList
        data={servers}
        renderItem={renderItem}
        keyExtractor={item => item.id} />
    </View>
  )
}

export default ServerList
