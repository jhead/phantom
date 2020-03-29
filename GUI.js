const childProcess = require('child_process');
var server = childProcess.fork('lib/server.js');
var api = childProcess.fork('lib/api.js');
