/**
 *
 * @constructor
 * @param {html} element - The HTML element
 * @param {function} onMessage - A function to be run when the user inputs a message
 * @param {options} options - The initial options for the Interface
 * 
 * @param {object} options.messageOptions - Options for messages
 * @param {object} options.messageOptions.text - Options for text
 * @param {string} options.messageOptions.text.defaultTextStyle - The defualt text style: 'your-class-name'
 * @param {object} options.messageOptions.tags - Options for author tags
 * @param {boolean} options.messageOptions.tags.enabled - If true, outputs the author of the message
 * @param {string} options.messageOptions.tags.defaultTagStyle - The defualt tag style: 'your-class-name'
 * @param {string} options.messageOptions.separators - 
 * @param {object} options.messageOptions.tags.tagStyles - Additional styles for tags: {author:'your-class-name', author2...}
 * 
 * 
 * 
 */
function Interface(element, onMessage, options){

	this.defaultOptions = { //The defualt options for an interface
		
		messageOptions: {
			text:{
				defaultTextStyle: null
			},
			tags:{
				enabled: true,
				defaultTagStyle: null,
				tagStyles:{} 
			},
			includeTime: true, //Shows the time of the message
			striped: true,	//Alternating message colors
			separators: false //Includes seperators between messages; either false, true or full	
		},

		consoleCommands: {
			enabled: true, //Allows the use of console commands
			deliminator: "//", //Deliminator phrase for console commands
			commands: { //List of console commands
				clearHistory: this.clearHistory
			} 
		},

		code: {
			usage: false, //False: no code evaluation, True: all text the user has entered is evaluated as js, tagged: code is run between between deliminator
			deliminator:'<<'//Deliminator phrase for code (will be reversed for the closing deliminator)
		},

		parrot: { 
			enabled: true, //Prints everything that the user inputs
			tag: 'You' //The author tag for the user - will look in authors for styling
		},

		listen:{
			enabled: false,  //Prints everything the browser console outputs
			tag: 'Console' //The author tag for the browser console - will look in authors for styling
		},

	}

	this.options = fullMerge(this.defaultOptions,options);
	this.onMessage = onMessage; 
	this.history = [];
	
	this.element = element;
	this.element.history = element.children[0].children[0].children[0];
	this.element.form = element.children[0].children[1].children[0];
	this.element.inputBox = this.element.form.children[0];
	this.element.inputBtn = this.element.form.children[1];
	
	this.element.form.addEventListener('submit', (e) => {
		e.preventDefault();
		this.getInput();
	});

	this.element.inputBtn.addEventListener('click',() => {this.getInput()});

}

function Message(e){

	this.default = {
		tag: null, //The author of the message - not required
		text: null, //The text of the message
		style: { //Overides any default styles
			block: null, //Inline or block style
			tagStyle: null, //Add css for the author '.your-class-name'
			textStyle: null //Add css for the text '.your-class-name'
		}
	}

	if(typeof e == 'string'){
		this.tag = "undefined"
		this.text = e
	} else {
		e = fullMerge(this.default,e);
		this.tag = "" + e.tag;
		this.text = "" + e.text;
		this.style = e.style;	
	}

	this.time = Date.now();
}




Interface.prototype.evalCode = function(m){ //evaluates any code in a message

	switch (this.options.code.usage){
		case true:
			m.text = evaluate(m.text);
		break;
		case 'tagged':
			if(m.text.includes(this.options.code.deliminator)){

				if(this.options.code.deliminator == reverseChars(this.options.code.deliminator)){
					m.text = m.text.split(this.options.code.deliminator);
				} else {
					m.text = multipleSplit(m.text,[this.options.code.deliminator, reverseChars(this.options.code.deliminator)]);
				}

				for(var i=0;i<m.text.length;i++){
					if(i % 2){
						m.text[i] = evaluate(m.text[i]);
					}
				}

				m.text = m.text.join('');
			}
		break;
	}

	return m;
}

Interface.prototype.evalCommands = function(m){ //evaluates console commands (can take arguments with //yourComand arg1 arg2..)

	var args = [],capture;

	m.text = m.text.split(" ").filter(a => a !== '');
	
	

	m.text.forEach(e=>{
		Object.keys(this.options.consoleCommands.commands).forEach((key,index) =>{
			if(e.substr(this.options.consoleCommands.deliminator.length, e.length) == key && e.substr(0, this.options.consoleCommands.deliminator.length) == this.options.consoleCommands.deliminator){
				try{
					//take in arguments
					if(this.options.consoleCommands.commands[key].length > 0){
						for(var i = 0; i < this.options.consoleCommands.commands[key].length; i++){
							if(typeof m.text[i+1] != 'undefined'){
								args.push(m.text[i+1]);
							} else {
								capture = i;
								throw {name:"Bad Arguments", message: key + " takes " + this.options.consoleCommands.commands[key].length + " arguments (" + 
									getParameters(this.options.consoleCommands.commands[key]).join(" ") +'), argument "' + 
									getParameters(this.options.consoleCommands.commands[key])[capture] + '" is missing.'};
							}
						}
					}

					this.options.consoleCommands.commands[key].bind(this)(...args);

				} catch(error){
					this.out(new Message({tag:"Console",text:"Console command failed, reason: " + error.message}));
				}
			}
		});
	});
}

Interface.prototype.copyInput = function(){ //Coppy's the current input

	var m = new Message();
	m.tag = this.options.parrot.tag;
	m.text = this.element.inputBox.value;

	return this.evalCode(m);
}

Interface.prototype.getInput = function(){ //Sends and clears the current input

	var m = this.copyInput();
	if(m.text != ''){
		this.element.inputBox.value = null;
		if(this.options.parrot.enabled == true) this.out(m);
		if(this.options.consoleCommands.enabled == true) this.evalCommands(m);
		this.onMessage(m);
	}

}

Interface.prototype.clearHistory = function(){ //Clears the history

	this.history = [];

	while (this.element.history.firstChild) {
		this.element.history.removeChild(this.element.history.firstChild);
	}

}

Interface.prototype.removeLast = function(number){ //Clears the history from the bottom
	
	if(typeof number != 'undefined'){
		while (number<0) {
			this.history.pop();
			this.element.history.removeChild(this.element.history.lastChild);
			number--;
		}
	} else {
		this.history.pop();
		this.element.history.removeChild(this.element.history.lastChild);
	}
	
}

Interface.prototype.removeFirst = function(){ //Clears the history from the top

	if(typeof number != 'undefined'){
		while (number<0) {
			this.history.shift();
			this.element.history.removeChild(this.element.history.firstChild);
			number--;
		}
	} else {
		this.history.shift();
		this.element.history.removeChild(this.element.history.firstChild);
	}

}

Interface.prototype.out = function(m){ //Outputs a message to the interface

	var html = document.createElement("div");
	html.classList.add("message");

	//add stripes
	if(this.options.messageOptions.striped && this.element.history.childElementCount % 2){
		html.classList.add("striped");
	}

	//add separators
	switch(this.options.messageOptions.separators){
		case 'full': 
			if(this.element.history.childElementCount == 0){
				this.element.history.style.marginTop += '10px';
				html.classList.add("separator-top");
			}

			html.classList.add("separator-bottom");
		break;
		case true:
			if(this.element.history.childElementCount != 0){
				html.classList.add("separator-top");
			}
		break;
	}

	//add tags
	if(this.options.messageOptions.tags.enabled){

		html.appendChild(document.createElement("div"));
		html.lastChild.classList.add("message-author");
		html.lastChild.appendChild(document.createTextNode(m.tag + ":"));

		if(m.style.tagStyle != null){
			html = addClasses(html, m.style.tagStyle);
		} else {
			if(this.options.messageOptions.tags.defaultTagStyle != null){
				html = addClasses(html, this.options.messageOptions.tags.defaultTagStyle);
			} else {
				if(Object.keys(this.options.messageOptions.tags.tagStyles).includes(m.tag)){
					html = addClasses(html, this.options.messageOptions.tags.tagStyles[m.tag]);
				}
			}
		}

	}
	
	//add the message
	html.appendChild(document.createElement("div"));
	html.lastChild.classList.add("message-text");

	evalLinks(m.text).forEach(e =>{
		html.lastChild.appendChild(e);
	});

	html.lastChild.normalize();

	if(m.style.textStyle != null){
		html = addClasses(html, m.style.textStyle);
	} else {
		if(this.options.messageOptions.tags.defaultTagStyle != null){
			html = addClasses(html, this.options.messageOptions.text.defaultTextStyle);
		} 
	}

	//add time
	if(this.options.messageOptions.includeTime) {

		html.appendChild(document.createElement("div"));
		html.lastChild.classList.add("message-time");
		html.lastChild.appendChild(document.createTextNode(
			addZero(new Date(m.time).getHours()) + ":" + addZero(new Date(m.time).getMinutes())
		));

	}

	this.history.push(m);

	//display
	this.element.history.appendChild(html);
	this.element.history.lastChild.scrollIntoView();
}

function reverseChars(str){ //Reverses given charecters (including open/close chars)

	var chars = ['[','{','(','<'], reverseChars = [']','}',')','>'];
	str = str.split('');

	for(var i = 0;i<chars.length;i++){
		for(var j =0;j<str.length;j++){
			str[j] = str[j].replace(chars[i],reverseChars[i]);
		}
	}
	
	return str.reverse().join('');
}


function multipleSplit(text,separators){ //splits on multiple separators

	var temp, output = [];
	separators.forEach(separator=>{
		
		if(output.length == 0){

			temp = text.split(separator);
			if(temp.length > 1)	output = temp;
			
		} else {
			for(var i = 0; i<output.length;i++){

				temp = output[i].split(separator);
				if(temp.length > 1){
					//merge the elements in
					output.splice(i, 1, ...temp);
					//update i
					i+= temp.length-1;
				}

			}
		}
	});
	
	return output;
}

function fullMerge(target, ...sources){ //merges two or more objects

	if(typeof target == 'undefined' || target == null) target = {};
	
	sources.forEach(source => {

		if(typeof source == 'undefined') source = {};
		for(var key in target){		
			if(source[key] != undefined){
				if(typeof target[key] == 'object' && target[key] != null){
					target[key] = fullMerge(target[key],source[key]);
				} else {
					if(typeof source[key] != 'null') target[key] = source[key];
				}
			}
		}

		for(var key in source) {
			if(target[key] == undefined) target[key] = source[key];
		}
	});

	return target;
}

function addClasses(html,classList){

	classList.split(" ").forEach(e => {
		html.lastChild.classList.add(e);
	});

	return html;
}

function addZero(i) { //adds zero for 24 hour times
	if (i < 10) {
	  i = "0" + i;
	}
	return i;
}

function evalLinks(string){ //transforms hyperlinks into real links
	
	var temp, output = [];

	string.split(' ').forEach(e=>{
		if(e.includes("http")){
			temp = document.createElement("a");
			temp.href = e;
			temp.appendChild(document.createTextNode(e)); 
		} else {
			temp = document.createTextNode(e)
		}

		output.push(temp);
	});

	var i = 1;

	while (i < output.length) {
		output.splice(i, 0, document.createTextNode(" "));
		i += 2;
	}

	return output;

}

function selectiveSplit(str,split){ //split that exludes charecters in quotation marks or apostrophies

	var currentPart = "", isInQuotes= false,isInApostrophe = false, output = [];

	for(var i = 0; i < str.length; i++){

		var char = str.charAt(i);
		
		if (char === split && !isInQuotes && !isInApostrophe){
			output.push(currentPart);
			currentPart = "";
		} else {
			currentPart += char;
		}

		if (char === '"') {
			isInQuotes = !isInQuotes;
		} 
		
		if (char == "'") {
			isInApostrophe = !isInApostrophe;
		}
	}

	if (currentPart) output.push(currentPart);

	return output;
}

const evaluate = (str) =>{ //eval that stores variables

	var operators = ['+', '-', '*', '/', '%'],coppy = str,output = [];	
	
	str = selectiveSplit(str,';').forEach(e =>{
		
		var pivot = null;
		e = e.trim().split("");	

		for(var i = 0;i < e.length;i++){
			if(e[i] == "=" && e[i-1] != "=" && e[i+1] != "="){
				if(operators.includes(e[i-1])){
					pivot = i-1;
				} else {
					pivot = i;
				}
			}
		}

		e = e.join("");

		if(pivot == null){

			try {
				output.push(eval(e));
			} catch(error){ 
				output.push(error.message);
			}	
					
		} else {
		
			if(e.substring(0,3) == "var"){
				type = e.substring(0,3);
			} else if(e.substring(0,5) == "const"){
				type = e.substring(0,5);
			}
			
			try {
				if(operators.includes(e[i-1])){
					this[e.substring(type.length,pivot).trim()] = eval(e.substring(pivot+2,e.length));
				} else {
					this[e.substring(type.length,pivot).trim()] = eval(e.substring(pivot+1,e.length));
				}
				output.push("");
			} catch(error){ 
				output.push(error.message);
			}	
		}
		
	});

	if(output.join(" ").trim() != ""){
		return output.join("").trim();
	}

	return coppy;

}

function getParameters(e){
	return e.toString().replace(/((\/\/.*$)|(\/\*[\s\S]*?\*\/)|(\s))/mg,'').match(/^function\s*[^\(]*\(\s*([^\)]*)\)/m)[1].split(/,/);
}