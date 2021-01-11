import React from 'react'
import {
  FlatList,
  View,
  Text
} from 'react-native'
import type { ListRenderItemInfo } from 'react-native'
import { Styles } from '../../styles'
import type { ServerEntry } from '../../services/Servers'

declare interface ServerListProps {
  servers: ServerEntry[]
}

const ServerList = ({ servers }: ServerListProps) => {
  const Item = ({ address, title }) => (
    <View style={Styles.serverItem}>
      <View style={Styles.serverItemTitle}>
        <Text style={Styles.serverItemName}>{ title }</Text>
        <Text style={Styles.serverItemDesc}>{ address }</Text>
      </View>
      <View style={Styles.serverItemStatusContainer}>
        <Text style={Styles.serverItemStatus}>O</Text>
      </View>
    </View>
  )

  const renderItem = ({ item }: ListRenderItemInfo<ServerEntry>) => (
    <Item
      key={ item.id }
      address={ item.prefs?.remoteServer }
      title={ item.name } />
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
