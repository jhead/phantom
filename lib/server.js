const open = require('open');
var express = require('express');
var path = require('path');
var app = express();

app.use(express.static('webApp'))

app.listen(555);
console.log('Starting server on port 555');


(async () => {
    // Opens the url in the default browser
    await open('http://localhost:555');
})();