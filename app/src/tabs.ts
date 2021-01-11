import { Colors } from './styles';
import type { LayoutBottomTabs, OptionsBottomTab } from 'react-native-navigation';

const defineTab = (text: string, icon: string, props: OptionsBottomTab = {}): OptionsBottomTab => {
  return Object.assign({
    text,
    textColor: Colors.text,
    iconColor: Colors.text,
    selectedTextColor: Colors.selectedText,
    selectedIconColor: Colors.selectedText,
    icon: {
      system: icon
    }
  }, props);
};

export default {
  id: 'BOTTOM_TABS_LAYOUT',
  children: [
    {
      stack: {
        id: 'TAB_SERVERS',
        children: [
          {
            component: {
              id: 'HOME_SCREEN',
              name: 'HOME_SCREEN'
            }
          }
        ],
        options: {
          bottomTab: defineTab('Servers', 'server.rack')
        }
      }
    },
    {
      stack: {
        id: 'TAB_SETTINGS',
        children: [
          {
            component: {
              id: 'HOME_SCREEN',
              name: 'HOME_SCREEN'
            }
          }
        ],
        options: {
          bottomTab: defineTab('Settings', 'gearshape')
        }
      }
    }
  ],
  options: {
    bottomTabs: {
      
    }
  }
} as LayoutBottomTabs;
