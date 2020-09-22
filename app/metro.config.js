const {
  applyConfigForLinkedDependencies,
} = require('@carimus/metro-symlinked-deps');

/**
 * Metro configuration for React Native
 * https://github.com/facebook/react-native
 *
 * @format
 */

module.exports = applyConfigForLinkedDependencies({
  transformer: {
    getTransformOptions: async () => ({
      transform: {
        experimentalImportSupport: false,
        inlineRequires: false,
      },
    }),
  },
});
