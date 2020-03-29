
// var child = require("child_process");
// var loader = child.spawn('python', ['r/loader.py', 'hansard', func]);

// loader.stdout.on('data', (data) => {
    
//     console.log(`${data}`);
    
 
// });

// loader.stderr.on('data', (data) => {
//     //console.log(`${data}`);
// });

// loader.on('close', (code) => {
//     //console.log(`loader terminated, code: ${code}`);
// });

function sleep(fun,time){
	setTimeout(function () {
		fun();
	}, time);
}

sleep(()=>{console.log('cool')},1000);