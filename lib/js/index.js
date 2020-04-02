//The front end code for the web app
var params = {
	server:null,
	boundIP:null,
	boundPort:null,
	timeOut:null
}, running = false,phantomHistory = [],


//An instanciation of Interface.js
c = new Interface(document.getElementById('interface').firstElementChild,m => {},{

	messageOptions: {
		striped:true,
		separators:false,
		tags:{
			tagStyles:{
				You:"client",
				Console:"host",
				Phantom:'phantom'
			}
		}
	},
	parrot:{
		enabled:true
	},
	code: {
		usage:'tagged'
	},
	consoleCommands:{
		deliminator: "-",
		commands:{ 
			server: function(adress){
				if(running){
					this.out(new Message({text:"Please close Phantom with -close before editing parameters" + time,tag:"Console"}));
				} else {
					this.out(new Message({text:"Setting server ip to " + adress,tag:"Console"}));
					if(params.server==null){
						sleep(()=>{
							c.out(new Message({text:"You can now start phantom with -start, or for more options use -help",tag:"Console"}));
						},1000);
					}
					params.server = adress;
				}
			},

			bind:function(ip){
				if(running){
					this.out(new Message({text:"Please close Phantom with -close before editing parameters" + time,tag:"Console"}));
				} else {
					this.out(new Message({text:"Binding phantom ip to" + ip,tag:"Console"}));
					params.boundIP = ip;
				}
				
			},

			bind_port:function(port){
				if(running){
					this.out(new Message({text:"Please close Phantom with -close before editing parameters" + time,tag:"Console"}));
				} else {
					this.out(new Message({text:"Binding phantom port to" + port,tag:"Console"}));
					params.boundPort = port;
				}
			},

			timeout:function(time){
				if(running){
					this.out(new Message({text:"Please close Phantom with -close before editing parameters" + time,tag:"Console"}));
				} else {
					this.out(new Message({text:"Setting timeout to " + time,tag:"Console"}));
					params.timeOut = time;
				}				
			},

			start:function(){
				if(params.server != null){
					this.out(new Message({text:"You can stop Phantom at any time with -stop",tag:"Console"}));
					post('start');
				} else {
					this.out(new Message({text:"Please enter the adress of the server.",tag:"Console"}));
				}
			},
			
			stop:function(){
				if(running){
					post('stop');
				} else {
					this.out(new Message({text:"The server is not currently running.",tag:"Console"}));
				}
			},

			restart:function(){
				if(running){
					post('restart')
				} else {
					this.out(new Message({text:"The server is not currently running.",tag:"Console"}));
				}
			},

			params:function(){
				this.out(new Message({text:params,tag:"Console"}));
			},

			help:function(){
				this.out(new Message({text:"List of commands:",tag:'Console'}));
				this.out(new Message({text:"-bind: Optional - IP address to listen on. Defaults to all interfaces. (default '0.0.0.0')"}));
				this.out(new Message({text:"-bind_port:	Optional - Port to listen on. Defaults to 0, which selects a random port. Note that phantom always binds to port 19132 as well, so both ports need to be open."}));
				this.out(new Message({text:"-timeout: Optional - Seconds to wait before cleaning up a disconnected client (default 60)"}));
				this.out(new Message({text:"-server: Required - Bedrock/MCPE server IP address and port (ex: 1.2.3.4:19132)"}));
				this.out(new Message({text:"-start: Starts Phantom."}));
				this.out(new Message({text:"-stop: Stops Phantom."}));
				this.out(new Message({text:"-restart: Restarts Phantom."}));
				this.out(new Message({text:"-params: Shows the current parameters of Phantom."}))
			}
		}
	}
});

//simple wait function
function sleep(fun,time){
	setTimeout(function () {
		fun();
	}, time);
}

//gets the current history/params from the api
function get(){
	fetch('/api')
	.then(
	  	function(response) {
			if (response.status !== 200) {
				console.log('Looks like there was a problem. Status Code: ' + response.status);
				return;
			}

			response.json().then(function(data) {
				running = data.running;
				output(data.history);
			});
	  	}
	)
	.catch(function(err) {
	  console.log(err);
	});
}

//Posts to the api
function post(type){

	console.log('Current params:');

	fetch('/api', {
		method: 'post',
		headers: {
			'Content-Type': 'application/json',
			"Accept": "application/json;"
		},
		body: JSON.stringify({parameters:params,command:type})
  	}).then(
		function(response){
			if (response.status !== 200) {
				console.log('Looks like there was a problem. Status Code: ' + response.status);
				return;
			}

			response.json().then(function(data) {
				console.log(data);
			});
		}
	  )
	  .catch(function(err) {
		console.log(err);
	  });
}

//merges the new history and outputs to the console
function output(h){
	if(phantomHistory != h){
		h.forEach((e,i)=>{
			if(phantomHistory[i] == undefined){
				console.log(e);
				c.out(new Message({text:e,tag:'Phantom'}));
				phantomHistory.push(h[i]);
			}
		});
	}
}

//Function that runs every second to get the latest history from Phantom
function checkServer() {
   	setTimeout(function () {
		if(params.server != null)get();
		checkServer();
   }, 1000)
}

window.addEventListener("beforeunload", function(e){
	post('close');

	return null;
});

window.addEventListener("load", function(){
	checkServer();
});

//Intro text
sleep(()=>{
	c.out(new Message({text:"Welcome to Phantom!",tag:"Console"}));
	sleep(()=>{
		c.out(new Message({text:'Please enter the IP and Port of the server that you want to connect to with -server',tag:'Console'}));
		sleep(()=>{
			c.out(new Message({text:'E.g. -server IP:Port',tag:'Console'}));
		},1000)
	},1000)
},500);