import { Navigation } from "react-native-navigation";
import HomeScreen from './src/screens/home';
import { Colors } from "./src/styles";
import bottomTabs from './src/tabs';
import PhantomMemberane from 'react-native-phantom-membrane';

PhantomMemberane.start()

Navigation.registerComponent('HOME_SCREEN', () => HomeScreen);

Navigation.events().registerAppLaunchedListener(() => {
  Navigation.setRoot({
    root: {
      stack: {
        children: [
          {
            component: {
              name: 'HOME_SCREEN'
            }
          }
        ]
      },
      bottomTabs
    }
  });
});

Navigation.setDefaultOptions({
  statusBar: {
    translucent: true,
    blur: true
  },
  topBar: {
    noBorder: true,
    background: {
      color: Colors.backgroundDark,
      // translucent: true,
      // blur: true
    },
    leftButtonColor: Colors.selectedText,
    rightButtonColor: Colors.selectedText
  },
  bottomTabs: {
    backgroundColor: 'black',
    hideShadow: true
  }
});
