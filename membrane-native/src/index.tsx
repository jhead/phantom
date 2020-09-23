import { NativeModules } from 'react-native';

type PhantomMembraneType = {
  start(): void;
};

const { PhantomMembrane } = NativeModules;

export default PhantomMembrane as PhantomMembraneType;
