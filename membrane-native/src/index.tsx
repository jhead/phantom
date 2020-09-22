import { NativeModules } from 'react-native';

type PhantomMembraneType = {
  multiply(a: number, b: number): Promise<number>;
};

const { PhantomMembrane } = NativeModules;

export default PhantomMembrane as PhantomMembraneType;
