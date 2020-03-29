var express = require('express');
var bodyParser = require('body-parser');
var childProcess = require('child_process');
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

// configure app to use bodyParser()
// this will let us get the data from a POST
app.use(bodyParser.urlencoded({ extended: true }));
app.use(bodyParser.json());

var router = express.Router();


router.get('/', function(req, res) {
    res.json({params:params,history:history,running:running});   
});

app.post('/',function(request,response){
    
    console.log(request.body);

    if(request.body.params != params){   
        params = request.body.var1;
        if(running){
            restart();
        }
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
    }
});

// all of our routes will be prefixed with /api
app.use('/', router);
app.listen(5555);
console.log('Starting api on port 5555 \n \n');

//neeed to verify the phantom type!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!

function start(){
    if(running == true){
        console.log('Error, phantom already running.');
    } else {
        console.log('Starting phantom... \n');
        history.push('Starting phantom...')
        running = true;
        // !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! use correct param
        phantom = childProcess.execFile('phantom-windows.exe',['-server', '147.135.177.107:25582' /*params.server*/]);

        phantom.stderr.on('data', function(data) {
            history.push(data);
            console.log(data);
        });

        phantom.stdout.on('data', function(data) {
            history.push(data);
            console.log(data);
        });
        
        phantom.on('close', function(code) {
            hisory.push('Phantom stopped.')
            console.log('Phantom closed. \n');
        });
    }
}

function stop(){
    if(phantom != null && running == true){
        phantom.kill();
    }
}

function restart(){
    stop();
    start();
}

start();

setTimeout(function(){
    stop();
}, 3000);