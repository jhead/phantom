/**
 * @format
 */
import { Navigation } from "react-native-navigation";
import App from './src/App';

const root = 'io.jxh.phantom.Home';
Navigation.registerComponent(root, () => App);

Navigation.events().registerAppLaunchedListener(() => {
    Navigation.setRoot({
        root: {
        stack: {
            children: [
            {
                component: {
                name: root
                }
            }
            ]
        }
        }
    });
});
