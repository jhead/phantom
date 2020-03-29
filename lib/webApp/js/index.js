//The front end code for the web app

var params = {
	server:null,
	boundIP:null,
	boundPort:null,
	timeOut:null
}, running = false,

c = new Interface(document.getElementById('interface').firstElementChild,m => {},{

	messageOptions: {
		striped:true,
		separators:false,
		tags:{
			tagStyles:{
				You:"client",
				Console:"host"
			}
		}
	},
	parrot:{
		enabled:false
	},
	code: {
		usage:'tagged'
	},
	consoleCommands:{
		deliminator: "-",
		commands:{ 
			server: function(adress){
				this.out(new Message({text:"Setting server ip to " + adress,tag:"Console"}));
				if(params.server==null){
					sleep(()=>{
						c.out(new Message({text:"You can now start phantom with -start, or for more options use -help",tag:"Console"}));
					},1000);
				}
				params.server = adress;
			},

			bind:function(ip){
				this.out(new Message({text:"Binding phantom ip to" + ip,tag:"Console"}));
				params.boundIP = ip;
			},

			bind_port:function(port){
				this.out(new Message({text:"Binding phantom port to" + port,tag:"Console"}));
				params.boundPort = port;
			},

			timeout:function(time){
				this.out(new Message({text:"Setting timeout to " + time,tag:"Console"}));
				params.timeOut = time;
			},

			start:function(){
				if(params.server != null){
					running = true;
				} else {
					this.out(new Message({text:"Please enter the adress of the server.",tag:"Console"}));
				}
			},
			
			stop:function(){
				if(running == true){

				} else {
					this.out(new Message({text:"The server is not currently running.",tag:"Console"}));
				}
			},

			restart:function(){
				if(running == true){

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

function sleep(fun,time){
	setTimeout(function () {
		fun();
	}, time);
}

sleep(()=>{
	c.out(new Message({text:"Welcome to Phantom!",tag:"Console"}));
	sleep(()=>{
		c.out(new Message({text:'Please enter the IP and Port of the server that you want to connect to with -server',tag:'Console'}));
		sleep(()=>{
			c.out(new Message({text:'E.g. -server IP:Port',tag:'Console'}));
		},1000)
	},1000)
},500);
