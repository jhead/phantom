import { StyleSheet } from 'react-native';

export const Colors = {
  backgroundDark: '#101024',
  foregroundDark: '#1c1c38',
  accent: '#4848f7',
  tabBackground: '#171021',
  text: 'rgba(255, 255, 255, 0.75)',
  selectedText: '#4848f7',
  black: '#000000',
  white: '#ffffff',
  lighter: '#ff0000'
};

export const Dimensions = {
  margin: 15,
  textMargin: 5,
  paddingVertical: 15,
  paddingHorizontal: 10,
  shadowRadius: 12,
  shadowOffset: {
    width: 0,
    height: 10
  },
  borderRadius: 5
}

export const Styles = StyleSheet.create({
  rootView: {
    flex: 1,
    backgroundColor: Colors.backgroundDark,
  },
  controlBar: {
    backgroundColor: Colors.foregroundDark,
    flexDirection: 'row',
    alignSelf: 'flex-end',
    marginTop: Dimensions.margin,
    marginHorizontal: Dimensions.paddingHorizontal,
    borderRadius: 25
  },
  controlBarLabel: {
    alignSelf: 'center',
    color: 'white',
    paddingHorizontal: 2 * Dimensions.paddingHorizontal,
  },
  controlBarSwitch: {
  },
  serverList: {
    flex: 1,
    flexDirection: 'column',
    paddingHorizontal: Dimensions.paddingHorizontal,
    paddingVertical: Dimensions.paddingVertical,
  },
  serverItem: {
    flex: 1,
    flexDirection: 'row',
    padding: 25,
    marginBottom: Dimensions.margin,
    backgroundColor: Colors.foregroundDark,
    borderRadius: Dimensions.borderRadius,
    shadowOffset: Dimensions.shadowOffset,
    shadowRadius: Dimensions.shadowRadius,
    shadowColor: 'black',
    shadowOpacity: 0.3
  },
  serverItemTitle: {
  },
  serverItemName: {
    color: Colors.text,
    marginBottom: Dimensions.textMargin,
    fontSize: 24
  },
  serverItemDesc: {
    color: Colors.text
  },
  serverItemStatusContainer: {
    alignContent: 'center'
  },
  serverItemStatus: {
    color: 'red'
  },
  sectionTitle: {
    fontSize: 24,
    fontWeight: '600',
    color: Colors.black,
  },
  highlight: {
    fontWeight: '700',
  }
});
