
var ip = null,c = new Interface(document.getElementById('interface').firstElementChild,m => {},{

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
		deliminator: "/",
		commands:{ 
			ip: function(x){
				this.out(new Message({text:"Setting server ip to: " + x,tag:"Console"}));
				if(ip==null){
					sleep(()=>{
						c.out(new Message({text:"You can now start phantom with /start, or for more options use /help",tag:"Console"}));
					},500);
				}
				ip = x;
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
		c.out(new Message({text:'Please enter the IP and Port of the server that you want to connect to with /ip',tag:'Console'}));
		sleep(()=>{
			c.out(new Message({text:'E.g. /ip serverIP:serverPort',tag:'Console'}));
		},1000)
	},1000)
},500);


