// Graphical user interface for phantom for people who are less technically skilled
// 
// To build into exe run the command pgk phantom-GUI.js
// For more info see: https://www.npmjs.com/package/pkg
//  
// The front end code is at lib/js/indexjs
// I have used my own front end library (Interface.js) to create the console
//
// Lib and phantom exe are intended to be included in the same file when running phantom-GUI

var express = require('express');
var bodyParser = require('body-parser');
var childProcess = require('child_process');
var open = require('open');
var app = express();

var params = {
	server:null,
	boundIP:null,
	boundPort:null,
	timeOut:null
},
running = false,
phantom = null,
history = [];


app.use(bodyParser.urlencoded({ extended: false }))
app.use(bodyParser.json())


var router = express.Router();

//Handles the get requests from the web page
router.get('/', function(req, res) {
    res.json({params:params,history:history,running:running});   
});

//Handles the post request from the web page
router.post('/',function(request,response){

    response.send(request.body);
      
    if(request.body.parameters != params){   
        params = request.body.parameters;
    }

    switch(request.body.command){
        case 'start':
            start();
        break;
        case 'stop':
            stop();
        break;
        case 'restart':
            restart();
        break;
        case 'close':
            close();
        break
    }
});

//Starts the server and api
app.use('/', express.static('lib'))
app.use('/api', router);
app.listen(555);
console.log('\n Starting api on port 555 \n');

(async () => {
    // Opens the url in the default browser
    await open('http://localhost:'+555);
})();

//Starts phantom
function start(){
    if(running != true){
        
        console.log('\n Starting phantom... \n');
        history.push('Starting phantom...')
        running = true;

        //get the correct os
        var os;
        switch(process.platform){
            case 'win32':
                os = 'windows.exe';
            break;
            case 'darwin':
                os = 'macos';
            break;
            case 'linux':
                os = 'linux'
            break;
        }

        //determine which parameters have been set by user
        var args = ['-server',params.server];

        if(params.boundIP != null){
            args.push('-bind');
            args.push(params.boundIP);
        }

        if(params.boundPort != null){
            args.push('-bind_port');
            args.push(params.boundPort);
        }

        if(params.timeOut != null){
            args.push('-timeout');
            args.push(params.timeOut);
        }

        //Launches phantom as a child process
        phantom = childProcess.execFile('phantom-' + os,args);

        phantom.stderr.on('data', function(data) {
            history.push(data);
            console.log(data);
        });

        phantom.stdout.on('data', function(data) {
            history.push(data);
            console.log(data);
        });
        
        phantom.on('close', function(code) {
            history.push('Phantom stopped.')
            console.log('Phantom closed.');
            running = false;
        });
    }
}

//Stops phantom
function stop(){
    if(phantom != null){
        phantom.kill();
    }
}

//Restarts Phantom
function restart(){
    if(running){
        stop();
        setTimeout(()=>{
            restart()
        }, 1000);
    } else {
        start();
    }
}

//Closes the GUI
function close(){
    if(running){
        stop();

        setTimeout(()=>{
            close()
        }, 1000);
    } else {
        process.exit();
    }
}