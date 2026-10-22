#!/usr/bin/env node
// Real CLI, Git worktrees, owned HTTP fixtures, cleanup commands and browser.
// Native records are seeded idle; no agent turn, credentials or remote traffic.
import fs from 'node:fs/promises';
import path from 'node:path';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';
import {findChrome,launchChrome} from './viewer-page-load-smoke.mjs';
import {buildCLI,createRunDir,makeRunCommand,startViewServer,stopProcess} from './support/view-smoke-harness.mjs';
const repo=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const cli=path.join(repo,'test/e2e/.bin',process.platform==='win32'?'archetipo.exe':'archetipo');
const timeout=20000,checks=[];
const ok=message=>{checks.push(message);console.log('✓ '+message);};
async function until(fn,label){const end=Date.now()+timeout;while(Date.now()<end){if(await fn())return;await delay(50);}throw new Error('Timeout: '+label);}
async function main(){
 const chrome=await findChrome();assert.ok(chrome,'Chrome required (ARCHETIPO_CHROME)');
 const dir=await createRunDir(path.join(repo,'test/workspaces/increment-deletion-smoke'));
 const sandbox=path.join(dir,'sandbox');await fs.mkdir(sandbox,{recursive:true});
 const env={...process.env,ARCHETIPO_DATA_DIR:repo,ARCHETIPO_STATE_DIR:path.join(dir,'state')};
 const run=makeRunCommand(env),items=[];let viewer,browser,page;
 try{
  await buildCLI(cli,repo,run);
  await run('init',cli,['init','--tool','codex','--connector','file','--yes'],{cwd:sandbox});
  await fs.appendFile(path.join(sandbox,'.archetipo/config.yaml'),'\nexecution:\n  default_provider:\n    id: codex\n    config:\n      command: '+JSON.stringify(path.join(repo,'test/e2e/support/fake-codex.mjs'))+'\n      model: gpt-fake\n');
  await fs.writeFile(path.join(sandbox,'hello.txt'),'baseline');
  await fs.writeFile(path.join(sandbox,'service.mjs'),`import fs from 'node:fs';import http from 'node:http';const root=process.cwd();const server=http.createServer((req,res)=>{res.end('owned');if(req.url==='/shutdown'){server.close(()=>process.exit(0));}});server.listen(0,'127.0.0.1',()=>fs.writeFileSync('.archetipo/environment/service.json',JSON.stringify({root,pid:process.pid,url:'http://127.0.0.1:'+server.address().port})));`);
  await fs.writeFile(path.join(sandbox,'setup.mjs'),`import fs from 'node:fs';import {spawn} from 'node:child_process';fs.mkdirSync('.archetipo/environment',{recursive:true});fs.writeFileSync('synthetic.data','owned fixture');const child=spawn(process.execPath,['service.mjs'],{detached:true,stdio:'ignore'});child.unref();`);
  await fs.writeFile(path.join(sandbox,'probe.mjs'),`import fs from 'node:fs';import {setTimeout as delay} from 'node:timers/promises';let ready=false;for(let i=0;i<100;i++){try{const state=JSON.parse(fs.readFileSync('.archetipo/environment/service.json'));if(state.root===process.cwd()&&(await fetch(state.url)).ok){ready=true;break;}}catch{}await delay(25);}if(!ready)process.exit(1);`);
  await fs.writeFile(path.join(sandbox,'cleanup.mjs'),`import fs from 'node:fs';const state=JSON.parse(fs.readFileSync('.archetipo/environment/service.json'));if(state.root!==process.cwd())throw Error('wrong owner');const response=await fetch(state.url+'/shutdown');if(!response.ok)throw Error('shutdown failed');fs.rmSync('synthetic.data');`);
  const recipeFile=path.join(dir,'environment.json');
  await fs.writeFile(recipeFile,JSON.stringify({version:1,inputs:['setup.mjs','probe.mjs','cleanup.mjs','service.mjs'],outputs:['synthetic.data'],prepare:[{name:'setup',argv:[process.execPath,'setup.mjs'],timeout_seconds:10}],probe:[{name:'probe',argv:[process.execPath,'probe.mjs'],timeout_seconds:10}],cleanup:[{name:'cleanup',argv:[process.execPath,'cleanup.mjs'],timeout_seconds:10}],checks:{}}));
  await run('configure',cli,['execution','environment','configure','--file',recipeFile],{cwd:sandbox});
  const git=async args=>(await run('git','git',args,{cwd:sandbox})).stdout.trim();
  await git(['init','-b','main']);await git(['config','user.name','Deletion smoke']);await git(['config','user.email','smoke@example.test']);
  await git(['add','hello.txt','service.mjs','setup.mjs','probe.mjs','cleanup.mjs','.archetipo/config.yaml','.archetipo/environment.json']);await git(['commit','-m','test: owned environment']);
  for(const [index,mode] of ['delete-keep','delete-remove','archive-keep','archive-remove','button'].entries()){
   const input=path.join(dir,'create-'+index+'.json');await fs.writeFile(input,JSON.stringify({title:mode,request:'Verify cleanup '+mode}));
   const record=JSON.parse((await run('create',cli,['increment','create','--file',input],{cwd:sandbox})).stdout).data;
   await run('prepare',cli,['execution','environment','prepare'],{cwd:record.workdir});
   const service=JSON.parse(await fs.readFile(path.join(record.workdir,'.archetipo/environment/service.json'),'utf8'));
   const id='conv-deletion-'+index,archived=mode.startsWith('delete'),now=new Date().toISOString();
   record.conversation_id=id;
   await fs.writeFile(path.join(record.workdir,'.archetipo/increment-local/increment.json'),JSON.stringify(record));
   await fs.mkdir(path.join(sandbox,'.archetipo/conversations'),{recursive:true});
   await fs.writeFile(path.join(sandbox,'.archetipo/conversations',id+'.json'),JSON.stringify({version:2,id,title:mode,increment_id:record.id,working_dir:record.workdir,provider_id:'codex',opened_at:now,last_message_at:now,message_count:0,archive:archived?'ARCHIVED':'OPEN',connection:'RELEASED',work:'IDLE',recovery:'RESUMABLE',session:{conversation_id:id,provider_id:'codex',environment:{working_dir:record.workdir,location:'local'},native:{kind:'codex.thread',id:'native-preserved-'+index}}}));
   await fs.writeFile(path.join(record.workdir,'untracked.txt'),'uncommitted outcome');
   items.push({id,mode,record,service});
  }
  viewer=await startViewServer(cli,sandbox,env,'/api/board');browser=await launchChrome(chrome,path.join(dir,'chrome'));page=await browser.newPage();
  const exceptions=[];page.on('Runtime.exceptionThrown',event=>exceptions.push(event.exceptionDetails));await page.send('Runtime.enable');
  await page.send('Page.navigate',{url:viewer.url});
  const choose=async choice=>{await page.waitFor('!!document.querySelector("[data-removal-dialog][open]")',timeout,'worktree cleanup choice');await page.evaluate(`document.querySelector('[data-removal-dialog][open] [data-removal-choice="${choice}"]').click()`);await page.waitFor('!document.querySelector("[data-removal-dialog]")',timeout,'choice dialog closed');};
  const open=async item=>{await page.waitFor(`!!document.querySelector('[data-conversation-id="${item.id}"]')`,timeout,'conversation in rail');await page.evaluate(`document.querySelector('[data-conversation-id="${item.id}"]').click()`);await page.waitFor(`document.querySelector('.conv-panel')?.dataset.conversationId===${JSON.stringify(item.id)} || !!document.querySelector('[data-increment-id="${item.record.id}"]')`,timeout,'increment conversation');};
  for(const item of items){
   if(item.mode.startsWith('delete')){
    await page.waitFor(`!!document.querySelector('[data-conversation-delete="${item.id}"]')`,timeout,'delete chat control');
    await page.evaluate(`document.querySelector('[data-conversation-delete="${item.id}"]').click()`);
   }else{
    await open(item);
    if(item.mode==='button'){
     await page.evaluate('document.querySelector("[data-increment-panel=open]").click()');
     await page.waitFor('!!document.querySelector("[data-increment-delete]")',timeout,'dedicated delete increment button');
     await page.evaluate('document.querySelector("[data-increment-delete]").click()');
    }else await page.evaluate('document.querySelector("[data-conversation-close-open]").click()');
   }
   await page.waitFor('!!document.querySelector("[data-removal-dialog]")',timeout,'explicit cleanup choice');
   if(item.mode==='delete-keep'){
    await choose('cancel');assert.equal((await fs.stat(item.record.workdir)).isDirectory(),true);
    await page.evaluate(`document.querySelector('[data-conversation-delete="${item.id}"]').click()`);
   }
   const keep=item.mode.endsWith('keep');
   await choose(keep?'keep':'delete');
   const endpoint=viewer.url+'/api/workspace/conversations/'+item.id;
   if(item.mode.startsWith('archive')){
    await until(async()=>{const value=await (await fetch(endpoint)).json();return value.session?.archive==='ARCHIVED';},'archived chat remains readable');
    const value=await (await fetch(endpoint)).json();assert.equal(value.session.session.native.id,item.id.replace('conv-deletion-','native-preserved-'));
   }else await until(async()=>!(await fetch(endpoint)).ok,'chat removed');
   if(keep){assert.equal((await fetch(item.service.url)).ok,true);assert.equal(await fs.readFile(path.join(item.record.workdir,'synthetic.data'),'utf8'),'owned fixture');}
   else{
    await until(async()=>!(await fs.stat(item.record.workdir).then(()=>true,()=>false)),'worktree removed');
    await until(async()=>{try{await fetch(item.service.url);return false;}catch{return true;}},'owned test service stopped');
    assert.equal(await git(['for-each-ref','--format=%(refname)','refs/heads/'+item.record.branch]),'');
    const receipt=JSON.parse(await fs.readFile(path.join(sandbox,'.archetipo/increment-archives',item.record.id,'deletion.json'),'utf8'));
    assert.equal(receipt.deleted,true);assert.ok((await fs.stat(receipt.archive_path)).size>0);
   }
   ok(item.mode+': scelta verificata nel browser, esito confermato su chat, servizio, dati, worktree e branch');
  }
  await page.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  const kept=items.find(item=>item.mode==='archive-keep');await open(kept);
  await page.evaluate('document.querySelector("[data-increment-panel=open]").click()');await page.waitFor('!!document.querySelector("[data-increment-delete]")',timeout,'archived increment delete control');
  await page.evaluate('document.querySelector("[data-increment-delete]").click()');await page.waitFor('!!document.querySelector("[data-removal-dialog]")',timeout,'mobile confirmation');
  assert.equal(await page.evaluate('Array.from(document.querySelectorAll("[data-removal-choice]")).every(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&r.top>=0&&r.bottom<=innerHeight;})'),true);
  await fs.writeFile(path.join(dir,'confirmation-mobile.png'),Buffer.from((await page.send('Page.captureScreenshot',{format:'png'})).data,'base64'));await page.send('Input.dispatchKeyEvent',{type:'keyDown',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});await page.send('Input.dispatchKeyEvent',{type:'keyUp',key:'Escape',code:'Escape',windowsVirtualKeyCode:27});
  await page.waitFor('!document.querySelector("[data-removal-dialog]")',timeout,'Escape cancels removal');
  assert.equal((await fetch(kept.service.url)).ok,true);
  assert.deepEqual(exceptions,[]);
  await fs.writeFile(path.join(dir,'summary.json'),JSON.stringify({status:'pass',checks},null,2));
  await fs.writeFile(path.join(dir,'report.html'),'<!doctype html><meta charset=utf-8><h1>Increment deletion: PASS</h1><ul>'+checks.map(message=>'<li>'+message+'</li>').join('')+'</ul>');console.log('Report: '+dir);
 }catch(error){await fs.writeFile(path.join(dir,'failure.json'),JSON.stringify({error:error.stack,viewer:viewer?.log(),page:await page?.evaluate('document.body.innerText').catch(()=>null)},null,2));throw error;}
 finally{
  await page?.close();await browser?.close();if(viewer)await stopProcess(viewer.child,run);
  for(const item of items){try{await fetch(item.service.url+'/shutdown');}catch{/* Already cleaned. */}}
 }
}
main().catch(error=>{console.error(error);process.exitCode=1;});
