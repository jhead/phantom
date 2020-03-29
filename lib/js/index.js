
var ip = null,c = new Interface(document.getElementById('interface').firstElementChild,m => {},{

	messageOptions: {
		// striped:false,
		// separators:'full',
		tags:{
			tagStyles:{
				You:"client",
				Console:"host"
			}
		}
	},
	code: {
		usage:'tagged'
	},
	consoleCommands:{
		deliminator: "/",
		commands:{ 
			ip: function(x){
				ip = x;
				console.log(x);
				this.out(new Message({text:"Setting server ip to: " + x,tag:"Console"}));
			},
			args: function(x,y,z){
				console.log(x,y,z);
				this.out(new Message({text:x,tag:"Console"}));
				this.out(new Message({text:y,tag:"Console"}));
				this.out(new Message({text:z,tag:"Console"}));
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
		c.out(new Message({text:'Please enter the IP and Port of the server with /ip serverIP:serverPort',tag:'Console'}));
	},1000)
},500);


