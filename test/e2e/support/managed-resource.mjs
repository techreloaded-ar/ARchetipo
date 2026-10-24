// Real external HTTP dependency adapter. No Docker, framework or credentials.
// Commands are finite; the server deliberately survives the creating command.
import fs from 'node:fs';
import path from 'node:path';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';
const owner=process.env.ARCHETIPO_RESOURCE_OWNER;
const name=process.env.ARCHETIPO_RESOURCE_NAME;
const port=Number(process.env.ARCHETIPO_PORT_DATABASE);
const record=path.join(process.env.ARCHETIPO_ENVIRONMENT_STATE,`owned-${name}.json`);
const read=()=>{try{return JSON.parse(fs.readFileSync(record,'utf8'));}catch(error){if(error.code==='ENOENT')return null;throw error;}};
const request=async(target,route='/')=>{
 try {const response=await fetch(`http://127.0.0.1:${target.port}${route}`,{headers:{'X-Owner':owner},signal:AbortSignal.timeout(400)});return await response.json();}
 catch{return null;}
};
const action=process.argv[2];
if(!owner || !name || !port)throw new Error('Missing owned resource context');
if(action==='serve'){
 let data=null;
 const server=http.createServer(async(req,res)=>{
  if(req.headers['x-owner']!==owner){res.writeHead(403);res.end('{}');return;}
  if(req.url==='/data' && req.method==='POST'){
   let body='';for await(const chunk of req)body+=chunk;data=JSON.parse(body);
  }
  res.setHeader('Content-Type','application/json');res.end(JSON.stringify({owner,name,data}));
  if(req.url==='/stop'){server.close(()=>process.exit(0));server.closeAllConnections();}
 });
 server.listen(port,'127.0.0.1',()=>{
  fs.writeFileSync(record,JSON.stringify({owner,name,port,pid:process.pid}));
 });
}else if(action==='start'){
 const previous=read();
 if(previous && previous.owner!==owner && await request(previous))throw new Error('Unrelated resource');
 if(previous?.owner===owner && (await request(previous))?.owner===owner)process.exit(0);
 const child=spawn(process.execPath,[fileURLToPath(import.meta.url),'serve'],{detached:true,stdio:'ignore',env:process.env});child.unref();
 const deadline=Date.now()+5000;
 while(Date.now()<deadline){const current=read();if(current?.owner===owner && (await request(current))?.owner===owner)process.exit(0);await delay(20);}
 throw new Error('Strict port binding or readiness failed');
}else{
 if(action==='stop' && fs.existsSync(path.join(process.env.ARCHETIPO_ENVIRONMENT_STATE,'fail-resource-stop')))throw new Error('Injected stop failure');
 const current=read();
 if(!current){if(action==='probe')throw new Error('Resource missing');process.exit(0);}
 if(current.owner!==owner)throw new Error('Owner mismatch');
 const response=await request(current);
 if(response && response.owner!==owner)throw new Error('Endpoint owner mismatch');
 if(action==='probe' && !response)throw new Error('Resource unavailable');
 if(action==='stop' && response){await request(current,'/stop');}
 if(action==='stopped' && response)throw new Error('Service still running');
}
