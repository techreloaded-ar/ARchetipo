#!/usr/bin/env node
// Real browser, CLI, viewer, owned app server and native Codex adapter. The
// agent alone is simulated; no credentials or non-local services are needed.
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import {spawn} from 'node:child_process';
import assert from 'node:assert/strict';
import {fileURLToPath} from 'node:url';
import {setTimeout as delay} from 'node:timers/promises';
import {findChrome,launchChrome} from './viewer-page-load-smoke.mjs';
import {buildCLI,createRunDir,makeRunCommand,startViewServer,stopProcess,readBody} from './support/view-smoke-harness.mjs';

const repo=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const cli=path.join(repo,'test/e2e/.bin',process.platform==='win32'?'archetipo.exe':'archetipo');
const checks=[];
const ok=text=>{checks.push(text);console.log('✓ '+text);};
const timeout=20000;
async function until(fn,label){const end=Date.now()+timeout;while(Date.now()<end){if(await fn())return;await delay(80);}throw new Error('Timeout: '+label);}

async function main(){
 if(process.platform==='win32'){console.log('SKIP: native fake uses a POSIX shebang');return;}
 const chrome=await findChrome();assert.ok(chrome,'Chrome required (ARCHETIPO_CHROME)');
 const dir=await createRunDir(path.join(repo,'test/workspaces/preview-view-smoke'));
 const sandbox=path.join(dir,'sandbox');await fs.mkdir(sandbox,{recursive:true});
 const received=[],queue=[];
 const control=http.createServer(async(req,res)=>{
  res.setHeader('Content-Type','application/json');
  if(req.url.startsWith('/next')){const pid=Number(new URL(req.url,'http://localhost').searchParams.get('pid'));const index=queue.findIndex(item=>!item.targetPid||item.targetPid===pid);res.end(JSON.stringify(index<0?{kind:'none'}:queue.splice(index,1)[0]));return;}
  if(req.url==='/received'){received.push(JSON.parse(await readBody(req)));res.end('{}');return;}
  res.writeHead(404);res.end('{}');
 });
 await new Promise(r=>control.listen(0,'127.0.0.1',r));
 const env={...process.env,ARCHETIPO_DATA_DIR:repo,ARCHETIPO_STATE_DIR:path.join(dir,'state'),FAKE_CODEX_CONTROL:`http://127.0.0.1:${control.address().port}`};
 const run=makeRunCommand(env);const standalone=[];let viewer,contender,browser,page;
 try{
  await buildCLI(cli,repo,run);
  await run('init',cli,['init','--tool','codex','--connector','file','--yes'],{cwd:sandbox});
  await fs.appendFile(path.join(sandbox,'.archetipo/config.yaml'),'\nexecution:\n  default_provider:\n    id: codex\n    config:\n      command: '+JSON.stringify(path.join(repo,'test/e2e/support/fake-codex.mjs'))+'\n      model: gpt-fake\n      sandbox: workspace-write\n');
  const app=path.join(sandbox,'server.mjs');
  await fs.writeFile(app,`import http from 'node:http';import fs from 'node:fs';fs.writeFileSync('server.pid',String(process.pid));for(let i=0;i<150;i++)console.log('Output fixture '+i);http.createServer((req,res)=>{res.setHeader('X-Checkout',process.cwd());res.setHeader('Content-Type','text/html');res.end('<!doctype html><title>Preview smoke</title><h1>Risultato</h1><p id=descrizione>Descrizione del risultato</p><a href="/seconda">Seconda pagina</a><button data-testid="salva" onclick="document.body.dataset.clicked=1">Salva</button><script>window.mountId=Math.random();</script>');}).listen(Number(process.argv[2]),'127.0.0.1');`);
  const recipe=path.join(dir,'recipe.json');
  await fs.writeFile(recipe,JSON.stringify({version:1,directory:'.',argv:[process.execPath,'server.mjs','{port}'],url:'http://127.0.0.1:{port}/',prepare_environment:true}));
  await run('preview-configure',cli,['execution','preview','configure','--file',recipe],{cwd:sandbox});
  const environmentRecipe=path.join(dir,'environment-recipe.json');
  await fs.copyFile(path.join(repo,'test/e2e/support/managed-resource.mjs'),path.join(sandbox,'managed-resource.mjs'));
  const resourceCommand=action=>({name:'dependency-'+action,argv:[process.execPath,'managed-resource.mjs',action],timeout_seconds:10});
  await fs.writeFile(environmentRecipe,JSON.stringify({version:1,ports:['DATABASE'],resources:[{name:'database',start:resourceCommand('start'),probe:resourceCommand('probe'),stop:resourceCommand('stop'),stopped:resourceCommand('stopped')}],inputs:['server.mjs','setup.mjs','managed-resource.mjs'],outputs:['dependency.ready'],prepare:[{name:'setup',argv:[process.execPath,'setup.mjs'],timeout_seconds:10}],probe:[{name:'ready',argv:[process.execPath,'-e',"if(require('node:fs').readFileSync('dependency.ready','utf8')!=='ready')process.exit(1)"],timeout_seconds:10}],checks:{}}));
  await fs.writeFile(path.join(sandbox,'setup.mjs'),"import fs from 'node:fs';fs.appendFileSync('setup-count','setup\\n');fs.writeFileSync('dependency.ready','ready');");
  await run('environment-configure',cli,['execution','environment','configure','--file',environmentRecipe],{cwd:sandbox});
  // Two real increments inherit committed recipes and helpers from main.
  // Their resources and setup receipts remain checkout-local.
  await run('git-init','git',['init','-b','main'],{cwd:sandbox});
  await run('git-name','git',['config','user.name','Preview smoke'],{cwd:sandbox});
  await run('git-email','git',['config','user.email','preview@example.test'],{cwd:sandbox});
  await run('git-add','git',['add','.archetipo/config.yaml','.archetipo/environment.json','.archetipo/preview.json','server.mjs','setup.mjs','managed-resource.mjs'],{cwd:sandbox});
  await run('git-baseline','git',['commit','-m','test: reusable preview setup'],{cwd:sandbox});
  const incrementInput=path.join(dir,'increment.json');
  await fs.writeFile(incrementInput,JSON.stringify({title:'Isolated preview',request:'Verify inherited setup'}));
  const incrementPreviews=[];
  for(let index=0;index<2;index++){
   const created=JSON.parse((await run('increment-create-'+index,cli,['increment','create','--file',incrementInput],{cwd:sandbox})).stdout).data;
   const prepared=JSON.parse((await run('increment-prepare-'+index,cli,['execution','preview','prepare'],{cwd:created.workdir})).stdout).data;
   assert.equal(prepared.preparation.passed,true);assert.equal(prepared.preparation.stable,true);assert.equal(prepared.preparation.reused,false);
   const child=spawn(cli,['execution','preview','run'],{cwd:created.workdir,env,stdio:['ignore','pipe','pipe']});standalone.push(child);
   let output='',errors='',readyView;
   child.stdout.on('data',chunk=>{output+=chunk;try{readyView=JSON.parse(output).data;}catch{/* Wait for complete envelope. */}});
   child.stderr.on('data',chunk=>{errors+=chunk;});
   child.on('error',error=>{errors+=error.message;});
   await until(()=>{if(child.exitCode!==null)throw new Error('Standalone preview exited: '+errors);return readyView?.state==='ready';},'standalone increment preview '+index);
   assert.equal(readyView.reused,false);assert.equal(readyView.preparation.reused,true);
   assert.equal((await fetch(readyView.url)).headers.get('x-checkout'),created.workdir);
   assert.equal(await fs.readFile(path.join(created.workdir,'setup-count'),'utf8'),'setup\n');
   incrementPreviews.push({created,view:readyView});
  }
  assert.notEqual(incrementPreviews[0].view.resources.ports.DATABASE,incrementPreviews[1].view.resources.ports.DATABASE,'increments share dependency port');
  assert.notEqual(incrementPreviews[0].view.recipe.url,incrementPreviews[1].view.recipe.url,'increments share app port');
  assert.notEqual(incrementPreviews[0].view.preparation.environment_id,incrementPreviews[1].view.preparation.environment_id,'increments share resource identity');
  assert.equal(await fs.stat(path.join(sandbox,'dependency.ready')).then(()=>true,()=>false),false,'increment setup mutated parent checkout');
  await stopProcess(standalone[0],run);
  assert.equal((await fetch(incrementPreviews[1].view.url)).headers.get('x-checkout'),incrementPreviews[1].created.workdir,'stopping one increment disturbed another');
  await stopProcess(standalone[1],run);
  for(const {created} of incrementPreviews){
   const resources=JSON.parse(await fs.readFile(path.join(created.workdir,'.archetipo/environment/resources.json'),'utf8'));
   assert(resources.resources.every(r=>r.state==='stopped'),'standalone stop left services active');
  }
  // A failed readiness probe must prevent launching the app, with durable logs.
  const broken=incrementPreviews[0].created.workdir;
  await fs.rm(path.join(broken,'server.pid'));
  const brokenRecipe=JSON.parse(await fs.readFile(path.join(broken,'.archetipo/environment.json'),'utf8'));
  brokenRecipe.probe=[{name:'failed',argv:[process.execPath,'-e','process.exit(9)'],timeout_seconds:10}];
  const brokenInput=path.join(dir,'broken-environment.json');await fs.writeFile(brokenInput,JSON.stringify(brokenRecipe));
  await run('configure-broken',cli,['execution','environment','configure','--file',brokenInput],{cwd:broken});
  const failed=await run('failed-preview-start',cli,['execution','preview','run'],{cwd:broken,allowFailure:true});
  assert.notEqual(failed.code,0);assert.match(failed.stdout+failed.stderr,/report.json/);
  assert.equal(await fs.stat(path.join(broken,'server.pid')).then(()=>true,()=>false),false,'failed setup launched server');
  ok('Due incrementi reali ereditano le ricette, preparano risorse proprie e partono insieme con porte diverse; un setup fallito impedisce l’avvio.');
  await run('publish-common',cli,['execution','environment','publish'],{cwd:sandbox});
  viewer=await startViewServer(cli,sandbox,env,'/api/board');
  const openedResponse=await fetch(viewer.url+'/api/workspace/conversations',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});
  const opened=await openedResponse.json();assert.equal(openedResponse.status,201,JSON.stringify(opened));
  const conversationID=opened.conversation.id;
  browser=await launchChrome(chrome,path.join(dir,'chrome'));page=await browser.newPage();
  const exceptions=[];page.on('Runtime.exceptionThrown',event=>exceptions.push(event.exceptionDetails));
  await page.send('Runtime.enable');await page.send('Page.enable');await page.send('Page.bringToFront');
  const contexts=new Map();page.on('Runtime.executionContextCreated',event=>{if(event.context.auxData?.isDefault)contexts.set(event.context.auxData.frameId,event.context.id);});
  // Keep delivery and session operations real. Add only a read-only increment
  // presentation to the conversation GET, to exercise its shared work panel.
  await page.send('Page.addScriptToEvaluateOnNewDocument',{source:`
   const originalFetch=window.fetch.bind(window);
   window.fetch=async(input,options={})=>{
    const response=await originalFetch(input,options);
    if(new URL(typeof input==='string'?input:input.url,location.href).pathname===${JSON.stringify('/api/workspace/conversations/'+conversationID)} && (options.method||'GET')==='GET' && response.ok){
     const value=await response.json();value.increment={id:'layout-fixture',title:'Risultato della lavorazione',status_label:'Da provare',request:'Una pagina da verificare.',stories:[{code:'US-001',title:'Consultare il risultato',status_label:'Completata',body:'Il risultato è navigabile nel browser.'}],actions:[]};
     return new Response(JSON.stringify(value),{status:response.status,headers:response.headers});
    }
    return response;
   };
  `});
  await page.send('Page.navigate',{url:viewer.url});
  await page.waitFor(`!!document.querySelector('[data-conversation-id="${conversationID}"]')`,timeout,'conversation in rail');
  await page.evaluate(`document.querySelector('[data-conversation-id="${conversationID}"]').click()`);
  await page.waitFor('!!document.querySelector("[data-conversation-preview]")',timeout,'preview control in free conversation');
  const endpoint=viewer.url+`/api/workspace/conversations/${conversationID}`;
  await page.evaluate('document.querySelector("[data-conversation-preview]").click()');
  await page.waitFor('!document.querySelector("[data-preview=start]").disabled',timeout,'configured preview ready to start');
  assert.equal(await fs.stat(path.join(sandbox,'server.pid')).then(()=>true,()=>false),false,'preview started before button');
  const turnsBeforeSetup=received.filter(e=>e.kind==='turn/start').length;
  assert.equal(await page.evaluate('!!document.querySelector("[data-preview=prepare]")'),false,'ordinary UI exposes redundant preparation');
  await fetch(endpoint+'/preview/prepare',{method:'POST'});
  await until(async()=>{const value=await (await fetch(endpoint+'/preview')).json();return value.state==='stopped' && value.preparation?.passed && value.preparation?.stable;},'deterministic preparation without server');
  assert.equal(await fs.stat(path.join(sandbox,'server.pid')).then(()=>true,()=>false),false,'prepare launched app');
  assert.equal(received.filter(e=>e.kind==='turn/start').length,turnsBeforeSetup,'prepare contacted agent');
  await page.waitFor('!document.querySelector("[data-preview=start]").disabled',timeout,'prepared preview start enabled');
  await page.evaluate('document.querySelector("[data-preview=start]").click()');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'injected picker handshake',()=>page.evaluate('document.querySelector(".preview-status").textContent'));
  const runtime=await (await fetch(endpoint+'/preview')).json();assert.equal(runtime.state,'ready');assert.equal(runtime.reused,false);
  assert.equal(runtime.preparation.passed,true);assert.equal(runtime.preparation.stable,true);assert.equal(runtime.preparation.reused,true);
  assert.equal(await fs.readFile(path.join(sandbox,'setup-count'),'utf8'),'setup\n','warm start repeated installation');
  assert.equal(JSON.parse(await fs.readFile(path.join(sandbox,'.archetipo/preview.json'),'utf8')).url,'http://127.0.0.1:{port}/','runtime persisted its assigned port');
  ok('Preparazione senza agente né server; avvio con riuso verificato e porta assegnata senza modificare la ricetta.');

  const tree=await page.send('Page.getFrameTree');let frameID=tree.frameTree.childFrames[0].frame.id;
  await until(()=>contexts.has(frameID),'preview execution context');
  const evaluateFrame=async expression=>{
   const result=await page.send('Runtime.evaluate',{expression,contextId:contexts.get(frameID),returnByValue:true,awaitPromise:true});
   if(result.exceptionDetails)throw new Error(JSON.stringify(result.exceptionDetails));return result.result.value;
  };
  const mount=await evaluateFrame('window.mountId');
  const visibleEditors=()=>page.evaluate('Array.from(document.querySelectorAll(".preview-comment-editor")).filter(e=>!e.hidden).length');
  const key=async(value)=>{
   const code=({Enter:13,ArrowUp:38,ArrowDown:40,Home:36,End:35}[value]);
   await page.send('Input.dispatchKeyEvent',{type:'keyDown',key:value,code:value,windowsVirtualKeyCode:code,nativeVirtualKeyCode:code,...(value==='Enter'?{text:'\r',unmodifiedText:'\r'}:{})});
   await page.send('Input.dispatchKeyEvent',{type:'keyUp',key:value,code:value,windowsVirtualKeyCode:code,nativeVirtualKeyCode:code});
  };
  const clickControl=async selector=>{
   const point=await page.evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2};})()`);
   await page.send('Input.dispatchMouseEvent',{type:'mousePressed',...point,button:'left',clickCount:1});
   await page.send('Input.dispatchMouseEvent',{type:'mouseReleased',...point,button:'left',clickCount:1});
  };
  const choose=async(note)=>{
   if(!await page.evaluate('document.querySelector("[data-preview=pick]").getAttribute("aria-pressed") === "true"'))await page.evaluate('document.querySelector("[data-preview=pick]").click()');
   await evaluateFrame(`document.querySelector(${JSON.stringify(note.selector || 'button')}).click()`);
   await page.waitFor('document.querySelectorAll(".preview-comment textarea").length === '+note.index,timeout,'selected element draft');
   assert.equal(await page.evaluate('document.querySelector("[data-preview=send]").disabled'),true,'empty draft prevents batch');
   assert.equal(await page.evaluate('document.querySelector("#browser-preview").dataset.panel'),'expanded');
   assert.equal(await page.evaluate('document.querySelector("[data-preview=comments]").getAttribute("aria-selected")'),'true');
   assert.equal(await visibleEditors(),1);
   assert.match(await page.evaluate('document.querySelector(".preview-comments-hint").textContent'),/vuoto/);
   await page.evaluate(`(()=>{const input=Array.from(document.querySelectorAll('.preview-comment textarea')).at(-1);input.value=${JSON.stringify(note.text)};input.dispatchEvent(new Event('input',{bubbles:true}));})()`);
  };
  await choose({index:1,text:'Rinomina in Conferma'});
  assert.equal(await evaluateFrame('document.body.dataset.clicked'),undefined,'picker triggered app action');
  assert.equal(await page.evaluate('document.querySelectorAll("[data-comment-save],[data-comment-next]").length'),0);
  assert.equal(received.some(e=>e.kind==='turn/start' && JSON.stringify(e.params).includes('Rinomina in Conferma')),false,'autosaving sent feedback');
  await page.evaluate('document.querySelector("[data-preview=collapse]").click()');
  await choose({index:2,text:'Aumenta lo spazio sopra questo pulsante',selector:'h1'});
  await page.evaluate('document.querySelector("[data-preview=hide]").click()');
  await choose({index:3,text:'Rendi questa descrizione più leggibile e sufficientemente lunga da richiedere il troncamento della riga compatta',selector:'#descrizione'});
  await choose({index:4,text:'Commento temporaneo da rimuovere',selector:'a'});
  await page.evaluate(`document.querySelector('[aria-label="Rimuovi commento 4"]').click()`);
  assert.equal(await page.evaluate('document.querySelectorAll(".preview-comment").length'),3);
  await page.evaluate('document.querySelectorAll(".preview-comment-open")[2].click()');
  assert.equal(await page.evaluate('document.querySelector(".preview-comment").getBoundingClientRect().height < 70'),true,'closed comment stays compact');
  assert.equal(await page.evaluate('document.querySelector(".preview-comment-editor").hidden'),true);
  assert.equal(await page.evaluate('document.querySelector(".preview-comment").innerText.includes("Pronto da inviare")'),false);
  const highlighted=()=>evaluateFrame('(()=>{const overlay=document.querySelector("[data-archetipo-highlight]");const r=overlay.getBoundingClientRect();return {display:overlay.style.display,number:overlay.textContent,width:r.width};})()');
  await until(async()=>(await highlighted()).number==='3','active comment highlights description');
  const hoverPoint=await page.evaluate('(()=>{const r=document.querySelectorAll(".preview-comment-open")[1].getBoundingClientRect();return {x:r.x+10,y:r.y+r.height/2};})()');
  await page.send('Input.dispatchMouseEvent',{type:'mouseMoved',...hoverPoint});
  await until(async()=>{const h=await highlighted();return h.display==='block' && h.number==='2';},'hover second component');
  assert.equal(await page.evaluate('document.querySelectorAll(".preview-comment-open")[1].getAttribute("aria-expanded")'),'false');
  assert.equal((await highlighted()).width,await evaluateFrame('document.querySelector("h1").getBoundingClientRect().width'));
  await page.evaluate('document.querySelectorAll(".preview-comment-open")[1].focus()');assert.equal(await page.evaluate('document.activeElement === document.querySelectorAll(".preview-comment-open")[1]'),true,"comment button focus before Enter");await page.send("Page.bringToFront");await key('Enter');
  await page.waitFor('document.activeElement === document.querySelectorAll(".preview-comment textarea")[1]',timeout,'keyboard opens second editor');
  await page.evaluate('(()=>{const t=document.activeElement;t.value="Aumenta lo spazio sopra questo pulsante — modificato";t.dispatchEvent(new Event("input",{bubbles:true}));})()');
  await page.evaluate('document.querySelectorAll(".preview-comment-open")[0].click()');
  assert.equal(await visibleEditors(),1);
  assert.match(await page.evaluate('document.querySelectorAll(".preview-comment textarea")[1].value'),/modificato/);
  assert.equal(await page.evaluate('document.querySelectorAll(".preview-comment-open")[1].title.includes("Preview smoke")'),true);
  assert.equal(await page.evaluate('document.querySelector(".preview-comment-editor").hidden'),false);
  assert.equal(await page.evaluate('document.querySelector(".preview-comment-excerpt").textContent'),'Rinomina in Conferma');
  await clickControl('[data-preview=fullscreen]');
  await page.waitFor('document.fullscreenElement === document.querySelector("#browser-preview")',timeout,'fullscreen preview');
  await page.evaluate('new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))');
  const footerReachable=async()=>assert.equal(await page.evaluate('(()=>{const f=document.querySelector(".preview-comments-footer").getBoundingClientRect(),b=document.querySelector(".preview-bottom").getBoundingClientRect();return f.top>=b.top && f.bottom<=b.bottom+1 && Array.from(document.querySelectorAll(".preview-comments-footer button")).every(e=>{const r=e.getBoundingClientRect();return r.width>0 && r.left>=0 && r.right<=innerWidth && r.top>=b.top && r.bottom<=b.bottom+1;});})()'),true,'comment actions remain outside the scrolling list and inside viewport');
  const geometry=()=>page.evaluate('(()=>{const s=document.querySelector(".preview-separator"),b=document.querySelector(".preview-bottom"),f=document.querySelector("iframe");return {height:b.getBoundingClientRect().height,frame:f.getBoundingClientRect().height,min:+s.getAttribute("aria-valuemin"),max:+s.getAttribute("aria-valuemax"),now:+s.getAttribute("aria-valuenow"),text:s.getAttribute("aria-valuetext")};})()');
  const resize=async dy=>{
   const pt=await page.evaluate('(()=>{const r=document.querySelector(".preview-separator").getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2};})()');
   await page.send('Input.dispatchMouseEvent',{type:'mousePressed',...pt,button:'left',clickCount:1});
   await page.send('Input.dispatchMouseEvent',{type:'mouseMoved',x:pt.x,y:pt.y+dy,button:'left',buttons:1});
   await page.send('Input.dispatchMouseEvent',{type:'mouseReleased',x:pt.x,y:pt.y+dy,button:'left',clickCount:1});
  };
  const firstSize=await geometry();await resize(-80);const resized=await geometry();assert.ok(resized.height>firstSize.height+60,JSON.stringify({firstSize,resized}));
  assert.equal(await page.evaluate('!!document.querySelector(".preview-drag-cover")'),false);
  await page.evaluate('document.querySelector(".preview-separator").focus()');await key('Home');let g=await geometry();assert.equal(g.now,g.min);assert.ok(Math.abs(g.height-g.min)<2);assert.match(g.text,/pixel/);await footerReachable();
  await key('ArrowUp');g=await geometry();assert.equal(g.now,g.min+20);
  await key('ArrowDown');assert.equal((await geometry()).now,g.min);
  await key('End');g=await geometry();assert.equal(g.now,g.max);assert.ok(g.frame>=130);
  await resize(70);const savedHeight=(await geometry()).height;
  await page.evaluate('document.querySelector("[data-preview=output]").click()');
  assert.equal(await page.evaluate('document.querySelector(".preview-comments").hidden'),true);
  assert.equal(await page.evaluate('document.querySelector(".preview-log").hidden'),false);
  assert.equal(await page.evaluate('document.querySelector(".preview-comments-count").textContent'),'3');
  await page.waitFor('document.querySelector(".preview-log pre").textContent.includes("Output fixture 149")',timeout,'real server log');
  const scroll=await page.evaluate('(()=>{const p=document.querySelector(".preview-log");p.scrollTop=70;return p.scrollTop;})()');assert.ok(scroll>0,'output independently scrollable');
  await page.evaluate('document.querySelector("[data-preview=collapse]").click()');
  assert.equal(await page.evaluate('document.querySelector("#browser-preview").dataset.panel'),'collapsed');
  assert.ok((await geometry()).height<=44);
  await page.evaluate('document.querySelector("[data-preview=output]").click()');assert.ok(Math.abs((await geometry()).height-savedHeight)<2);
  assert.equal(await page.evaluate('document.querySelector(".preview-log").scrollTop'),scroll);
  await page.evaluate('document.querySelector("[data-preview=hide]").click()');
  assert.equal(await page.evaluate('document.querySelector(".preview-bottom").hidden'),true);
  assert.equal(await page.evaluate('document.querySelector("[data-preview=reopen]").hidden'),false);
  await page.evaluate('document.querySelector("[data-preview=reopen]").click()');assert.ok(Math.abs((await geometry()).height-savedHeight)<2);
  assert.equal(await evaluateFrame('window.mountId'),mount,'panel changes reloaded iframe');
  const fullscreenImage=await page.send('Page.captureScreenshot',{format:'png'});await fs.writeFile(path.join(dir,'preview-fullscreen.png'),Buffer.from(fullscreenImage.data,'base64'));
  await clickControl('[data-preview=fullscreen]');await page.waitFor('!document.fullscreenElement',timeout,'exit fullscreen');
  assert.equal(await page.evaluate('document.querySelector("[data-preview=output]").getAttribute("aria-selected")'),'true');
  await page.evaluate('document.querySelector("[data-preview=comments]").click()');
  assert.match(await page.evaluate('document.querySelectorAll(".preview-comment textarea")[1].value'),/modificato/);
  ok('AC1–19,21,24: tre righe compatte, editor unico, hover e tastiera, resize reale, tab/stati e fullscreen senza reload o perdita di bozze.');
  // Do not substitute a timer for the poll: wait for an actual HTTP chat read.
  let polls=0;page.on('Network.requestWillBeSent',e=>{if(e.request.url.startsWith(endpoint+'?'))polls++;});await page.send('Network.enable');
  await until(()=>polls>0,'conversation poll with preview mounted');
  assert.equal(await evaluateFrame('window.mountId'),mount,'chat polling reloaded the preview');
  ok('Avvio esplicito del server, selezione HTML senza azionare il pulsante e iframe stabile durante il polling.');
  await clickControl('[data-inspector-expand]');
  assert.equal(await page.evaluate('document.getElementById("workspace-primary").classList.contains("inspector-expanded")'),true);
  assert.equal(await evaluateFrame('window.mountId'),mount,'expansion remounted preview');
  await page.evaluate(`document.querySelector('[data-inspector-tab="details"]').click()`);
  await page.waitFor('!document.querySelector("#inspector-details").hidden && !!document.querySelector(".conv-increment-panel")',timeout,'increment in shared inspector');
  await page.evaluate(`document.querySelector('[data-inspector-tab="preview"]').click()`);
  await page.waitFor('!document.querySelector("#browser-preview").hidden',timeout,'return to mounted preview');
  assert.equal(await evaluateFrame('window.mountId'),mount,'changing inspector tabs remounted preview');
  assert.equal(await page.evaluate('document.getElementById("workspace-primary").classList.contains("inspector-expanded")'),true,'changing tabs lost expansion');
  await clickControl('[data-inspector-expand]');
  await clickControl('[data-work-panel-toggle]');
  await page.waitFor('document.getElementById("workspace-inspector").hidden',timeout,'hide shared preview panel');
  await clickControl('[data-work-panel-toggle]');
  await page.waitFor('!document.getElementById("browser-preview").hidden',timeout,'reopen last preview tab');
  assert.equal(await evaluateFrame('window.mountId'),mount,'closing and reopening panel remounted preview');

  assert.equal(await page.evaluate('document.querySelectorAll(".preview-comment textarea").length'),3);
  const drag=async(id,dx)=>{
   const before=await page.evaluate(`(()=>{const r=document.getElementById('${id}').getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+Math.min(80,r.height/2)};})()`);
   await page.send('Input.dispatchMouseEvent',{type:'mousePressed',x:before.x,y:before.y,button:'left',clickCount:1});
   await page.send('Input.dispatchMouseEvent',{type:'mouseMoved',x:before.x+dx,y:before.y,button:'left',buttons:1});
   await page.send('Input.dispatchMouseEvent',{type:'mouseReleased',x:before.x+dx,y:before.y,button:'left',clickCount:1});
  };
  const dimensions=()=>page.evaluate('({rail:document.getElementById("workspace-conversations").getBoundingClientRect().width,right:document.getElementById("workspace-inspector").getBoundingClientRect().width})');
  const initialColumns=await dimensions();await drag('rail-splitter',50);const afterRail=await dimensions();assert.ok(afterRail.rail>initialColumns.rail+40);
  await drag('inspector-splitter',-70);const afterRight=await dimensions();assert.ok(afterRight.right>afterRail.right+50);
  await page.evaluate('document.getElementById("rail-toggle").click()');
  assert.equal(await page.evaluate('document.getElementById("workspace-shell").classList.contains("rail-collapsed")'),true);
  assert.equal(await page.evaluate('document.getElementById("rail-toggle").getAttribute("aria-expanded")'),'false');
  await page.evaluate('document.getElementById("rail-toggle").click()');
  assert.ok(Math.abs((await dimensions()).rail-afterRail.rail)<2);
  const desktopImage=await page.send('Page.captureScreenshot',{format:'png'});await fs.writeFile(path.join(dir,'preview-desktop.png'),Buffer.from(desktopImage.data,'base64'));
  ok('Colonne ridimensionate con trascinamento, elenco richiudibile e schede Preview/Incremento senza perdita di pagina o commenti.');

  const killOwned=async()=>{const pid=Number(await fs.readFile(path.join(sandbox,'server.pid'),'utf8'));process.kill(pid,'SIGKILL');await until(async()=>(await (await fetch(endpoint+'/preview')).json()).state==='error','owned app error state');};
  const restartOwned=async()=>{await page.evaluate('document.querySelector("[data-preview=start]").click()');await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'owned app restarted');frameID=(await page.send('Page.getFrameTree')).frameTree.childFrames[0].frame.id;await until(()=>contexts.has(frameID),'restarted frame context');};
  await page.evaluate('document.querySelector("[data-preview=collapse]").click()');await killOwned();
  await page.waitFor('!document.querySelector(".preview-error-badge").hidden',timeout,'error badge in collapsed panel');
  assert.equal(await page.evaluate('document.querySelector("#browser-preview").dataset.panel'),'collapsed');
  assert.equal(await page.evaluate('document.querySelector("[data-preview=comments]").getAttribute("aria-selected")'),'true');
  await restartOwned();
  await page.evaluate('document.querySelector("[data-preview=comments]").click();document.querySelectorAll(".preview-comment-open")[1].click()');
  await page.waitFor('document.activeElement === document.querySelectorAll(".preview-comment textarea")[1]',timeout,'editing before server failure');
  await killOwned();await page.waitFor('!document.querySelector(".preview-error-badge").hidden',timeout,'error badge while editing');
  assert.equal(await page.evaluate('document.activeElement === document.querySelectorAll(".preview-comment textarea")[1]'),true,'server error stole editor focus');
  assert.equal(await page.evaluate('document.querySelector("[data-preview=comments]").getAttribute("aria-selected")'),'true');
  await restartOwned();
  ok('AC22–23: errori del vero processo owned segnalati in ridotto e durante modifica senza cambiare tab o focus.');
  await page.evaluate('document.querySelector("[data-preview=close]").click();document.querySelector("[data-conversation-preview]").click()');
  assert.equal(await page.evaluate('document.querySelectorAll(".preview-comment textarea").length'),3);
  await page.evaluate('document.querySelector("[data-preview=send]").click()');
  await until(()=>received.some(e=>e.kind==='turn/start' && JSON.stringify(e.params).includes('Rinomina in Conferma')),'native comment delivery');
  const delivered=received.find(e=>e.kind==='turn/start' && JSON.stringify(e.params).includes('Rinomina in Conferma'));
  const text=JSON.stringify(delivered.params);assert.match(text,/modificato/);assert.match(text,/descrizione più leggibile/);assert.match(text,/salva/);assert.match(text,/Preview smoke/);
  assert.equal(received.filter(e=>e.kind==='thread/start').length,1,'feedback opened a second native session');
  await page.waitFor('document.querySelectorAll(".preview-comment textarea").length === 0',timeout,'submitted drafts cleared');
  ok('Tre commenti inviati insieme alla stessa sessione nativa con testo, pagina e identità dell’elemento.');
  // A failed send must retain the complete draft.
  await choose({index:1,text:'Commento da conservare'});
  // A second real viewer owns the native conversation after release. The
  // original UI remains on its draft and receives the real ownership refusal.
  const released=await fetch(endpoint,{method:'DELETE'});assert.equal(released.status,200);
  contender=await startViewServer(cli,sandbox,env,'/api/board');
  const contenderEndpoint=contender.url+`/api/workspace/conversations/${conversationID}`;
  const taken=await fetch(contenderEndpoint+'/resume',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message:'Verifica esclusione ownership'})});assert.equal(taken.status,201,await taken.text());
  await page.evaluate('document.querySelector("[data-preview=send]").click()');
  await page.waitFor('document.querySelector(".preview-comments-hint").textContent.includes("another View")',timeout,'real ownership rejection visible in Comments');
  assert.equal(await page.evaluate('document.querySelector(".preview-comment textarea").value'),'Commento da conservare');
  assert.equal(await page.evaluate('document.querySelector(".preview-comments").hidden'),false);
  const relinquished=await fetch(contenderEndpoint,{method:'DELETE'});assert.equal(relinquished.status,200);
  const resumed=await fetch(endpoint+'/resume',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({message:'Riprendi la verifica'})});assert.equal(resumed.status,201,await resumed.text());
  ok('Un invio rifiutato conserva i commenti e mostra il motivo.');
  // Reload loses in-memory controller state, but must retain browser drafts.
  await page.send('Page.reload');
  await page.waitFor('!!document.querySelector("[data-conversation-preview]")',timeout,'conversation after reload');
  await page.evaluate('document.querySelector("[data-conversation-preview]").click()');
  await page.waitFor('document.querySelector(".preview-comment textarea")?.value === "Commento da conservare"',timeout,'draft restored after page reload');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'picker after page reload');
  frameID=(await page.send('Page.getFrameTree')).frameTree.childFrames[0].frame.id;
  await until(()=>contexts.has(frameID),'restored preview execution context');
  await page.evaluate('document.querySelector(".preview-comment-open").click()');
  await until(async()=>{const value=await evaluateFrame('document.querySelector("[data-archetipo-highlight]").style.display');return value==='block';},'component association restored after reload');
  const restoredColumns=await dimensions();assert.ok(Math.abs(restoredColumns.rail-afterRail.rail)<2);assert.ok(Math.abs(restoredColumns.right-afterRight.right)<2);
  ok('La riapertura della pagina conserva i commenti non inviati e le larghezze scelte.');
  const otherResponse=await fetch(viewer.url+'/api/workspace/conversations',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});assert.equal(otherResponse.status,201);const otherID=(await otherResponse.json()).conversation.id;
  await page.waitFor(`!!document.querySelector('[data-conversation-id="${otherID}"]')`,timeout,'second conversation rail');
  await page.evaluate(`document.querySelector('[data-conversation-id="${otherID}"]').click()`);
  await page.waitFor('document.querySelectorAll(".preview-comment").length === 0',timeout,'new conversation has no inherited drafts');
  await page.evaluate(`document.querySelector('[data-conversation-id="${conversationID}"]').click()`);
  await page.waitFor('document.querySelector(".preview-comment textarea")?.value === "Commento da conservare"',timeout,'original conversation draft isolated');
  await page.evaluate('(()=>{if(!document.querySelector("#browser-preview").checkVisibility())document.querySelector("[data-conversation-preview]").click();})()');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'original preview after switch');
  frameID=(await page.send('Page.getFrameTree')).frameTree.childFrames[0].frame.id;await until(()=>contexts.has(frameID),'frame after conversation switch');
  assert.equal(await visibleEditors(),1);
  ok('Le conversazioni isolano richieste ed editor attivo.');
  await page.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await page.waitFor('document.querySelector("#browser-preview").getBoundingClientRect().width < 400',timeout,'mobile preview');
  assert.equal(await page.evaluate('document.documentElement.scrollWidth > innerWidth'),false);await footerReachable();
  assert.equal(await page.evaluate('Array.from(document.querySelectorAll(".preview-tabs button")).every(e=>{const r=e.getBoundingClientRect();return r.left>=0 && r.right<=innerWidth && r.width>0;})'),true,'mobile tabs reachable');
  const image=await page.send('Page.captureScreenshot',{format:'png'});await fs.writeFile(path.join(dir,'preview-mobile.png'),Buffer.from(image.data,'base64'));
  await evaluateFrame('location.href="/seconda"');
  await until(async()=>{try{return await evaluateFrame('location.pathname')==='/seconda';}catch{return false;}},'navigate preview to another page');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'picker on second page');
  await page.evaluate('document.querySelector(".preview-comment-open").click();document.querySelector(".preview-comment-open").click()');
  await page.waitFor('document.querySelector(".preview-comment-reference").textContent.includes("altra pagina")',timeout,'reference to another page explained');
  assert.equal(await evaluateFrame('document.querySelector("[data-archetipo-highlight]").style.display'),'none');
  await evaluateFrame('location.href="/"');
  await until(async()=>{try{return await evaluateFrame('location.pathname')==='/';}catch{return false;}},'return to commented page');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'picker on original page');
  await evaluateFrame('document.querySelector("button").remove()');
  await page.evaluate('document.querySelector(".preview-comment-open").click();document.querySelector(".preview-comment-open").click()');
  await page.waitFor('document.querySelector(".preview-comment-reference").textContent.includes("non è più presente")',timeout,'missing component explained');
  assert.equal(await evaluateFrame('document.querySelector("[data-archetipo-highlight]").style.display'),'none');
  ok('Riferimenti ripristinati dopo reload; nessuna evidenziazione errata su altre pagine o componenti rimossi.');
  const failStop=path.join(sandbox,'.archetipo/environment/fail-resource-stop');
  await fs.writeFile(failStop,'fail stop');
  await page.evaluate('document.querySelector("[data-preview=stop]").click()');
  await until(async()=>{const state=await (await fetch(endpoint+'/preview')).json();return state.state==='error' && state.resources.resources.some(r=>r.state==='error');},'failed cleanup remains visible and registered');
  await fs.rm(failStop);
  await until(async()=>!(await page.evaluate('document.querySelector("[data-preview=stop]").disabled')),'cleanup retry enabled');
  await page.evaluate('document.querySelector("[data-preview=stop]").click()');
  await page.waitFor('!document.querySelector("[data-preview=start]").disabled',timeout,'preview stopped');
  const pid=Number(await fs.readFile(path.join(sandbox,'server.pid'),'utf8'));
  await until(()=>{try{process.kill(pid,0);return false;}catch{return true;}},'owned app process terminated');
  assert.equal(JSON.parse(await fs.readFile(path.join(sandbox,'.archetipo/environment/resources.json'),'utf8')).resources[0].state,'stopped');
  ok('Preview e commenti leggibili a 390px; arresto verificato di applicazione e dipendenza esterna.');
  // Hard viewer death closes the private supervisor pipe and its app tree.
  const started=await fetch(endpoint+'/preview',{method:'POST'});assert.equal(started.status,202);
  await until(async()=>{const state=await (await fetch(endpoint+'/preview')).json();return state.state==='ready';},'second app startup');
  const newPid=Number(await fs.readFile(path.join(sandbox,'server.pid'),'utf8'));assert.notEqual(newPid,pid);
  viewer.child.kill('SIGKILL');
  await until(()=>{try{process.kill(newPid,0);return false;}catch{return true;}},'server cleanup on hard viewer death');
  await until(async()=>JSON.parse(await fs.readFile(path.join(sandbox,'.archetipo/environment/resources.json'),'utf8')).resources.every(r=>r.state==='stopped'),'external dependency cleanup on hard viewer death');
  ok('Anche la terminazione forzata del viewer chiude applicazione e servizi esterni posseduti.');
  // The standalone environment run uses the same independent lifetime owner.
  const environmentRun=spawn(cli,['execution','environment','run'],{cwd:sandbox,env,stdio:['ignore','pipe','pipe']});standalone.push(environmentRun);
  let environmentOutput='',environmentError='';
  environmentRun.stdout.on('data',chunk=>{environmentOutput+=chunk;});environmentRun.stderr.on('data',chunk=>{environmentError+=chunk;});
  await until(()=>{if(environmentRun.exitCode!==null)throw new Error(environmentError);try{return JSON.parse(environmentOutput).data.passed;}catch{return false;}},'standalone environment run ready');
  const conflicting=await run('active-lifetime-conflict',cli,['execution','environment','prepare'],{cwd:sandbox,allowFailure:true});
  assert.notEqual(conflicting.code,0);assert.match(conflicting.stderr,/E_CONFLICT/);
  environmentRun.kill('SIGKILL');
  await until(async()=>JSON.parse(await fs.readFile(path.join(sandbox,'.archetipo/environment/resources.json'),'utf8')).resources.every(r=>r.state==='stopped'),'standalone run death cleans external dependency');
  ok('Stop fallito conservato e riprovato; environment run standalone esclusivo e cleanup anche dopo SIGKILL.');
  // First use has one Preview button and a distinct durable technical session.
  const managedQualification=JSON.parse((await run('managed-candidate-qualification',cli,['execution','environment','qualify'],{cwd:sandbox})).stdout).data;
  assert.equal(managedQualification.passed,true);
  assert.deepEqual(managedQualification.reports.map(report=>report.reused),[false,false,true]);
  await run('managed-candidate-cleanup',cli,['execution','environment','cleanup'],{cwd:sandbox});
  assert(JSON.parse(await fs.readFile(path.join(sandbox,'.archetipo/environment/resources.json'),'utf8')).resources.every(resource=>resource.state==='stopped'));
  ok('La qualificazione usa anche risorse gestite reali, rilascia il lifetime e conserva stop verificabile.');
  // The fake agent chooses configuration; CLI, preparation and startup are real.
  {
   const bootstrap=path.join(dir,'bootstrap');await fs.mkdir(bootstrap);
   await run('bootstrap-init',cli,['init','--tool','codex','--connector','file','--yes'],{cwd:bootstrap});
   await fs.appendFile(path.join(bootstrap,'.archetipo/config.yaml'),'\nexecution:\n  default_provider:\n    id: codex\n    config:\n      command: '+JSON.stringify(path.join(repo,'test/e2e/support/fake-codex.mjs'))+'\n      model: gpt-fake\n      sandbox: workspace-write\n');
   await fs.copyFile(app,path.join(bootstrap,'server.mjs'));
   for(const args of [['init','-b','main'],['config','user.name','Tests'],['config','user.email','tests@example.test'],['add','.archetipo/config.yaml','server.mjs'],['commit','-m','initial']])await run('bootstrap-git','git',args,{cwd:bootstrap});
   const early=JSON.parse((await run('early-increment',cli,['increment','create','--file',incrementInput],{cwd:bootstrap})).stdout).data;
   viewer=await startViewServer(cli,bootstrap,{...env,FAKE_CODEX_UNIQUE_THREADS:'1'},'/api/board');
   const product=await (await fetch(viewer.url+'/api/workspace/conversations',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})).json();
   const productID=product.conversation.id,productEndpoint=viewer.url+'/api/workspace/conversations/'+productID;
   await page.send('Page.navigate',{url:viewer.url});
   await page.waitFor(`!!document.querySelector('[data-conversation-id="${productID}"]')`,timeout,'bootstrap conversation');
   await page.evaluate(`document.querySelector('[data-conversation-id="${productID}"]').click()`);
   await page.waitFor('!!document.querySelector("[data-conversation-preview]")',timeout,'bootstrap preview control');
   await page.evaluate('document.querySelector("[data-conversation-preview]").click()');
   await page.waitFor('!document.querySelector("[data-preview=start]").disabled',timeout,'first-use Preview enabled');
   assert.equal(await page.evaluate('!!document.querySelector("[data-preview=setup], [data-preview=prepare]")'),false);
   await page.evaluate('document.querySelector("[data-preview=start]").click()');
   let technicalID,technicalStart;
   await until(async()=>{const state=await (await fetch(productEndpoint+'/preview')).json();technicalID=state.environment?.conversation_id;technicalStart=received.find(e=>e.kind==='turn/start'&&(e.params.input||[]).some(i=>i.text?.includes('/archetipo-environment')));return technicalID&&technicalStart;},'separate technical setup session');
   assert.notEqual(technicalID,productID);
   const ordinaryIndex=await (await fetch(viewer.url+'/api/workspace/conversations')).json();
   assert(!ordinaryIndex.conversations.some(c=>c.id===technicalID),'technical session leaked into ordinary rail');
   const technical=await (await fetch(viewer.url+'/api/workspace/conversations/'+technicalID)).json();
   assert.notEqual(technical.conversation.id,productID);
   await fs.mkdir(path.join(bootstrap,'scripts/archetipo'),{recursive:true});
   await fs.writeFile(path.join(bootstrap,'scripts/archetipo/prepare.mjs'),"import fs from 'node:fs';fs.appendFileSync('setup-count','setup\\n');fs.writeFileSync('dependency.ready','ready');");
   const setupInput=path.join(dir,'bootstrap-environment.json');
   await fs.writeFile(setupInput,JSON.stringify({version:1,inputs:['scripts/archetipo/prepare.mjs'],outputs:['dependency.ready'],prepare:[{name:'setup',argv:[process.execPath,'scripts/archetipo/prepare.mjs'],timeout_seconds:10}],probe:[{name:'ready',argv:[process.execPath,'-e',"if(!require('node:fs').existsSync('dependency.ready'))process.exit(1)"],timeout_seconds:10}],checks:{}}));
   await run('bootstrap-configure',cli,['execution','environment','configure','--candidate','--file',setupInput],{cwd:bootstrap});
   await run('bootstrap-preview',cli,['execution','preview','configure','--candidate','--file',recipe],{cwd:bootstrap});
   const observed=JSON.parse((await run('bootstrap-capabilities',cli,['execution','environment','capabilities','--tool',process.execPath],{cwd:bootstrap})).stdout).data;
   assert.equal(observed.external_network.state,'unknown');assert.equal(observed.loopback_binding.state,'available');
   const qualification=JSON.parse((await run('bootstrap-qualify',cli,['execution','environment','qualify','--asset','scripts/archetipo/prepare.mjs'],{cwd:bootstrap})).stdout).data;
   assert.equal(qualification.passed,true);assert.deepEqual(qualification.reports.map(report=>report.reused),[false,false,true]);
   assert.equal(await fs.stat(path.join(bootstrap,'.archetipo/environment-recipe.json')).then(()=>true,()=>false),false,'candidate was published before qualification');
   await run('bootstrap-publish',cli,['execution','environment','publish','--qualified','--asset','scripts/archetipo/prepare.mjs'],{cwd:bootstrap});
   queue.push({targetPid:technicalStart.pid,kind:'emit',method:'item/agentMessage/delta',params:{threadId:technical.session.session.native.id,turnId:'turn-1',itemId:'setup-output',delta:'BUILD_ONLY_IN_TECHNICAL_SESSION'}});
   queue.push({targetPid:technicalStart.pid,kind:'emit',method:'turn/completed',params:{threadId:technical.session.session.native.id,turn:{id:'turn-1',status:'completed'}}});
   await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'first-use configuration continues automatically into preview');
   const productHistory=await (await fetch(productEndpoint)).json();
   assert(!JSON.stringify(productHistory.events).includes('BUILD_ONLY_IN_TECHNICAL_SESSION'),'technical log polluted product conversation');
   const technicalHistory=await (await fetch(viewer.url+'/api/workspace/conversations/'+technicalID)).json();
   assert(JSON.stringify(technicalHistory.events).includes('BUILD_ONLY_IN_TECHNICAL_SESSION'),'technical log was not durable');
   assert.equal(await fs.readFile(path.join(bootstrap,'setup-count'),'utf8'),'setup\nsetup\n');
   const startsBefore=received.filter(e=>e.kind==='turn/start').length;
   const inherited=JSON.parse((await run('early-increment-preparation',cli,['execution','environment','prepare'],{cwd:early.workdir})).stdout).data;
   assert(inherited.passed&&inherited.stable);assert.equal(await fs.readFile(path.join(early.workdir,'setup-count'),'utf8'),'setup\n');
   const later=JSON.parse((await run('later-increment',cli,['increment','create','--file',incrementInput],{cwd:bootstrap})).stdout).data;
   const laterPrepared=JSON.parse((await run('later-increment-preparation',cli,['execution','environment','prepare'],{cwd:later.workdir})).stdout).data;
   assert(laterPrepared.passed&&laterPrepared.stable);
   assert.equal(received.filter(e=>e.kind==='turn/start').length,startsBefore,'inheritance reconfigured through agent');
   const another=await (await fetch(viewer.url+'/api/workspace/conversations',{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'})).json();
   const shared=await (await fetch(viewer.url+'/api/workspace/conversations/'+another.conversation.id+'/preview',{method:'POST'})).json();
   const original=await (await fetch(productEndpoint+'/preview')).json();assert.equal(shared.url,original.url,'same checkout launched duplicate preview');
   await fetch(productEndpoint+'/preview',{method:'DELETE'});
   ok('Capacità osservate e candidata qualificata con due preparazioni e riuso prima della pubblicazione. Un solo Preview configura in una sessione tecnica durevole e avvia automaticamente; nessun log nella chat di sviluppo. Ricetta e helper riusati da incrementi già aperti e nuovi, senza agente; due chat sullo stesso checkout condividono il server.');
  }
  assert.deepEqual(exceptions,[],'uncaught browser exceptions');
  await fs.writeFile(path.join(dir,'summary.json'),JSON.stringify({status:'pass',checks},null,2));
  await fs.writeFile(path.join(dir,'report.html'),`<!doctype html><meta charset="utf-8"><h1>Preview smoke: PASS</h1><ul>${checks.map(text=>'<li>'+text+'</li>').join('')}</ul>`);
  console.log('Report: '+dir);
 }catch(error){
  await fs.writeFile(path.join(dir,'failure.json'),JSON.stringify({error:error.stack,received,viewer:viewer?.log(),page:await page?.evaluate('document.body.innerText').catch(()=>null),focus:await page?.evaluate('({active:document.activeElement.outerHTML,editors:Array.from(document.querySelectorAll(".preview-comment-editor")).map(e=>e.hidden)})').catch(()=>null)},null,2));
  throw error;
 }finally{
  for(const child of standalone)if(child.exitCode===null && child.signalCode===null)await stopProcess(child,run);
  await page?.close();await browser?.close();if(viewer?.child.exitCode===null && viewer?.child.signalCode===null)await stopProcess(viewer.child,run);
  if(contender?.child.exitCode===null && contender?.child.signalCode===null)await stopProcess(contender.child,run);
  control.closeAllConnections();await new Promise(resolve=>control.close(resolve));
 }
}
main().catch(error=>{console.error(error);process.exitCode=1;});
