#!/usr/bin/env node
// Optional integration against an explicitly supplied, already installed Next.
// No dependency installation, AI credentials or changes to the source project.
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {spawn} from 'node:child_process';
import {parseArgs} from 'node:util';
import {fileURLToPath} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';
import assert from 'node:assert/strict';
import {buildCLI,createRunDir,makeRunCommand,stopProcess} from './support/view-smoke-harness.mjs';

const {values}=parseArgs({options:{'node-modules':{type:'string'}}});
assert(values['node-modules'],'Usage: npm run test:next-preview-process-smoke -- --node-modules /absolute/path/to/node_modules');
assert.notEqual(process.platform,'win32','This integration exercises POSIX process groups');
const modules=await fs.realpath(values['node-modules']);
const next=JSON.parse(await fs.readFile(path.join(modules,'next/package.json'),'utf8'));
const repo=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const dir=await createRunDir(path.join(repo,'test/workspaces/next-preview-process-smoke'));
const project=path.join(dir,'project');await fs.mkdir(project);
const bin=path.join(dir,'archetipo');
const env={...process.env,ARCHETIPO_DATA_DIR:repo,ARCHETIPO_STATE_DIR:path.join(dir,'state'),NEXT_TELEMETRY_DISABLED:'1'};
const run=makeRunCommand(env);
const results=[];let active;
const unrelated=http.createServer((_request,response)=>response.end('unrelated'));
await new Promise(resolve=>unrelated.listen(0,'127.0.0.1',resolve));
const unrelatedURL=`http://127.0.0.1:${unrelated.address().port}`;
async function until(predicate,label){const deadline=Date.now()+45000;while(Date.now()<deadline){if(await predicate())return;await delay(40);}throw new Error('Timeout: '+label);}
async function portClosed(url){try{await fetch(url,{signal:AbortSignal.timeout(500)});return false;}catch{return true;}}
try{
 await buildCLI(bin,repo,run);
 await run('init',bin,['init','--tool','codex','--connector','file','--yes'],{cwd:project});
 await fs.symlink(modules,path.join(project,'node_modules'),'dir');
 await fs.mkdir(path.join(project,'pages'));
 await fs.writeFile(path.join(project,'pages/index.js'),"export default function Page(){return 'Next process ownership test';}\n");
 await fs.writeFile(path.join(project,'package.json'),JSON.stringify({private:true,scripts:{dev:'node launcher.cjs'}}));
 await fs.writeFile(path.join(project,'launcher.cjs'),`const {spawn}=require('node:child_process');const fs=require('node:fs');const child=spawn(process.execPath,['node_modules/next/dist/bin/next','dev','--hostname','127.0.0.1','--port',process.argv[2]],{stdio:'inherit'});fs.writeFileSync('next.pid',String(child.pid));child.on('exit',code=>process.exit(code??1));setInterval(()=>{if(fs.existsSync('exit-launcher'))process.exit(0);},20);`);
 const recipe=path.join(dir,'preview.json');
 await fs.writeFile(recipe,JSON.stringify({version:1,directory:'.',argv:['npm','run','dev','--','{port}'],url:'http://127.0.0.1:{port}/'}));
 await run('configure',bin,['execution','preview','configure','--file',recipe],{cwd:project});
 for(const mode of ['graceful-stop','cli-stop','owner-sigkill','launcher-exit']){
  let output='',errors='';
  active=spawn(bin,['execution','preview','run'],{cwd:project,env,stdio:['ignore','pipe','pipe']});
  active.stdout.on('data',chunk=>{output+=chunk;});active.stderr.on('data',chunk=>{errors+=chunk;});
  await until(()=>{if(active.exitCode!==null)throw new Error(errors);try{return JSON.parse(output).data.state==='ready';}catch{return false;}},mode+' ready');
  const view=JSON.parse(output).data;
  assert.match(await (await fetch(view.recipe.url)).text(),/Next process ownership test/);
  if(mode==='cli-stop')await run('stop-active-next',bin,['execution','preview','stop'],{cwd:project});
  else if(mode==='launcher-exit')await fs.writeFile(path.join(project,'exit-launcher'),'exit');
  else active.kill(mode==='owner-sigkill'?'SIGKILL':'SIGTERM');
  await until(()=>active.exitCode!==null||active.signalCode!==null,mode+' owner exits');
  await until(()=>portClosed(view.recipe.url),mode+' Next port closes');
  await until(async()=>JSON.parse(await fs.readFile(path.join(project,'.archetipo/environment/preview-process.json'),'utf8')).state==='stopped',mode+' retained verified stop');
  const observed=JSON.parse((await run(mode+'-status',bin,['execution','preview','status'],{cwd:project})).stdout).data;
  assert.equal(observed.server.state,'stopped');assert.equal(observed.server.owner_active,false);
  assert.equal(await (await fetch(unrelatedURL)).text(),'unrelated');
  results.push({mode,passed:true,url:view.recipe.url,exit:active.exitCode,signal:active.signalCode});
  console.log('PASS '+mode+' (Next '+next.version+')');
  await fs.writeFile(path.join(dir,mode+'.log'),output+'\n'+errors);
  active=null;
 }
 await fs.writeFile(path.join(dir,'summary.json'),JSON.stringify({next:next.version,results},null,2));
 console.log('Report: '+dir);
}finally{
 if(active){
  await stopProcess(active,run);
  // Failure cleanup of this test's freshly recorded child only.
  try{process.kill(Number(await fs.readFile(path.join(project,'next.pid'),'utf8')),'SIGTERM');}catch{/* Already reaped. */}
 }
 await new Promise(resolve=>unrelated.close(resolve));
}
