#!/usr/bin/env node
// Real browser, CLI, viewer, owned app server and native Codex adapter. The
// agent alone is simulated; no credentials or non-local services are needed.
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import net from 'node:net';
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
async function freePort(){const s=net.createServer();await new Promise(r=>s.listen(0,'127.0.0.1',r));const port=s.address().port;await new Promise(r=>s.close(r));return port;}

async function main(){
 if(process.platform==='win32'){console.log('SKIP: native fake uses a POSIX shebang');return;}
 const chrome=await findChrome();assert.ok(chrome,'Chrome required (ARCHETIPO_CHROME)');
 const dir=await createRunDir(path.join(repo,'test/workspaces/preview-view-smoke'));
 const sandbox=path.join(dir,'sandbox');await fs.mkdir(sandbox,{recursive:true});
 const received=[],queue=[];
 const control=http.createServer(async(req,res)=>{
  res.setHeader('Content-Type','application/json');
  if(req.url.startsWith('/next')){res.end(JSON.stringify(queue.shift()||{kind:'none'}));return;}
  if(req.url==='/received'){received.push(JSON.parse(await readBody(req)));res.end('{}');return;}
  res.writeHead(404);res.end('{}');
 });
 await new Promise(r=>control.listen(0,'127.0.0.1',r));
 const env={...process.env,ARCHETIPO_DATA_DIR:repo,ARCHETIPO_STATE_DIR:path.join(dir,'state'),FAKE_CODEX_CONTROL:`http://127.0.0.1:${control.address().port}`};
 const run=makeRunCommand(env);let viewer,browser,page;
 try{
  await buildCLI(cli,repo,run);
  await run('init',cli,['init','--tool','codex','--connector','file','--yes'],{cwd:sandbox});
  await fs.appendFile(path.join(sandbox,'.archetipo/config.yaml'),'\nexecution:\n  default_provider:\n    id: codex\n    config:\n      command: '+JSON.stringify(path.join(repo,'test/e2e/support/fake-codex.mjs'))+'\n      model: gpt-fake\n      sandbox: workspace-write\n');
  const port=await freePort();
  const app=path.join(sandbox,'server.mjs');
  await fs.writeFile(app,`import http from 'node:http';import fs from 'node:fs';fs.writeFileSync('server.pid',String(process.pid));http.createServer((req,res)=>{res.setHeader('Content-Type','text/html');res.end('<!doctype html><title>Preview smoke</title><h1>Risultato</h1><a href="/seconda">Seconda pagina</a><button data-testid="salva" onclick="document.body.dataset.clicked=1">Salva</button><script>window.mountId=Math.random();</script>');}).listen(${port},'127.0.0.1');`);
  const recipe=path.join(dir,'recipe.json');
  await fs.writeFile(recipe,JSON.stringify({version:1,directory:'.',argv:[process.execPath,app],url:`http://127.0.0.1:${port}/`}));
  await run('preview-configure',cli,['execution','preview','configure','--file',recipe],{cwd:sandbox});
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
  await page.evaluate('document.querySelector("[data-preview=start]").click()');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'injected picker handshake',()=>page.evaluate('document.querySelector(".preview-status").textContent'));
  const runtime=await (await fetch(endpoint+'/preview')).json();assert.equal(runtime.state,'ready');assert.equal(runtime.reused,false);
  const tree=await page.send('Page.getFrameTree');let frameID=tree.frameTree.childFrames[0].frame.id;
  await until(()=>contexts.has(frameID),'preview execution context');
  const evaluateFrame=async expression=>{
   const result=await page.send('Runtime.evaluate',{expression,contextId:contexts.get(frameID),returnByValue:true,awaitPromise:true});
   if(result.exceptionDetails)throw new Error(JSON.stringify(result.exceptionDetails));return result.result.value;
  };
  const mount=await evaluateFrame('window.mountId');
  const choose=async(note)=>{
   if(!await page.evaluate('document.querySelector("[data-preview=pick]").getAttribute("aria-pressed") === "true"'))await page.evaluate('document.querySelector("[data-preview=pick]").click()');
   await evaluateFrame(`document.querySelector(${JSON.stringify(note.selector || 'button')}).click()`);
   await page.waitFor('document.querySelectorAll(".preview-comment textarea").length === '+note.index,timeout,'selected element draft');
   await page.evaluate(`(()=>{const input=Array.from(document.querySelectorAll('.preview-comment textarea')).at(-1);input.value=${JSON.stringify(note.text)};input.dispatchEvent(new Event('input',{bubbles:true}));})()`);
  };
  await choose({index:1,text:'Rinomina in Conferma'});
  assert.equal(await evaluateFrame('document.body.dataset.clicked'),undefined,'picker triggered app action');
  assert.equal(await page.evaluate('document.querySelector("[data-preview=send]").disabled'),false,'nonempty autosaved comment is ready');
  assert.equal(await page.evaluate('document.querySelectorAll("[data-comment-save],[data-comment-next]").length'),0,'redundant save controls');
  assert.equal(received.some(e=>e.kind==='turn/start' && JSON.stringify(e.params).includes('Rinomina in Conferma')),false,'autosaving sent feedback');
  await page.evaluate('document.querySelector("[data-preview=add]").click()');
  await choose({index:2,text:'Aumenta lo spazio sopra questo pulsante',selector:'h1'});
  const highlighted=()=>evaluateFrame('(()=>{const overlay=document.querySelector("[data-archetipo-highlight]");const r=overlay.getBoundingClientRect();return {display:overlay.style.display,number:overlay.textContent,x:r.x,y:r.y,width:r.width};})()');
  await until(async()=>{const value=await highlighted();return value.display==='block' && value.number==='2';},'focused comment highlights heading');
  const heading=await evaluateFrame('document.querySelector("h1").getBoundingClientRect().width');
  assert.equal((await highlighted()).width,heading,'focus highlighted wrong element');
  await page.evaluate('document.querySelector(".preview-comment").dispatchEvent(new MouseEvent("mouseenter"))');
  await until(async()=>{const value=await highlighted();return value.number==='1' && value.display==='block';},'hover highlights first component');
  assert.equal((await highlighted()).width,await evaluateFrame('document.querySelector("button").getBoundingClientRect().width'));
  await page.evaluate('document.querySelector(".preview-comment").dispatchEvent(new MouseEvent("mouseleave"))');
  await until(async()=>(await highlighted()).number==='2','leaving hover restores focused component');
  await page.evaluate('document.activeElement.blur()');
  await until(async()=>(await highlighted()).display==='none','blur clears component highlight');
  // Enter through a real user gesture: fullscreen requires transient activation.
  const clickControl=async selector=>{
   const point=await page.evaluate(`(()=>{const r=document.querySelector(${JSON.stringify(selector)}).getBoundingClientRect();return {x:r.x+r.width/2,y:r.y+r.height/2};})()`);
   await page.send('Input.dispatchMouseEvent',{type:'mousePressed',...point,button:'left',clickCount:1});
   await page.send('Input.dispatchMouseEvent',{type:'mouseReleased',...point,button:'left',clickCount:1});
  };
  await clickControl('[data-preview=fullscreen]');
  await page.waitFor('document.fullscreenElement === document.querySelector("#browser-preview")',timeout,'fullscreen preview');
  assert.equal(await evaluateFrame('window.mountId'),mount,'fullscreen reloaded the iframe');
  assert.equal(await page.evaluate('document.querySelectorAll(".preview-comment[data-complete=true]").length'),2);
  const fullscreenImage=await page.send('Page.captureScreenshot',{format:'png'});await fs.writeFile(path.join(dir,'preview-fullscreen.png'),Buffer.from(fullscreenImage.data,'base64'));
  await clickControl('[data-preview=fullscreen]');
  await page.waitFor('!document.fullscreenElement',timeout,'exit fullscreen');
  await page.evaluate('document.querySelector(".preview-comment textarea").focus()');
  await until(async()=>(await highlighted()).number==='1','editing highlights matching component');
  assert.equal(await page.evaluate('document.querySelector("[data-preview=send]").disabled'),false,'editing does not require manual save');
  ok('Schermo intero senza ricaricare; commenti salvati automaticamente e componenti evidenziati su hover e modifica.');
  // Do not substitute a timer for the poll: wait for an actual HTTP chat read.
  let polls=0;page.on('Network.requestWillBeSent',e=>{if(e.request.url.startsWith(endpoint+'?'))polls++;});await page.send('Network.enable');
  await until(()=>polls>0,'conversation poll with preview mounted');
  assert.equal(await evaluateFrame('window.mountId'),mount,'chat polling reloaded the preview');
  ok('Avvio esplicito del server, selezione HTML senza azionare il pulsante e iframe stabile durante il polling.');
  await page.evaluate(`document.querySelector('[data-inspector-tab="details"]').click()`);
  await page.waitFor('!document.querySelector("#inspector-details").hidden && !!document.querySelector(".conv-increment-panel")',timeout,'increment in shared inspector');
  await page.evaluate(`document.querySelector('[data-inspector-tab="preview"]').click()`);
  await page.waitFor('!document.querySelector("#browser-preview").hidden',timeout,'return to mounted preview');
  assert.equal(await evaluateFrame('window.mountId'),mount,'changing inspector tabs remounted preview');
  assert.equal(await page.evaluate('document.querySelectorAll(".preview-comment textarea").length'),2);
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

  await page.evaluate('document.querySelector("[data-preview=close]").click();document.querySelector("[data-conversation-preview]").click()');
  assert.equal(await page.evaluate('document.querySelectorAll(".preview-comment textarea").length'),2);
  await page.evaluate('document.querySelector("[data-preview=send]").click()');
  await until(()=>received.some(e=>e.kind==='turn/start' && JSON.stringify(e.params).includes('Rinomina in Conferma')),'native comment delivery');
  const delivered=received.find(e=>e.kind==='turn/start' && JSON.stringify(e.params).includes('Rinomina in Conferma'));
  const text=JSON.stringify(delivered.params);assert.match(text,/Aumenta lo spazio/);assert.match(text,/salva/);assert.match(text,/Preview smoke/);
  assert.equal(received.filter(e=>e.kind==='thread/start').length,1,'feedback opened a second native session');
  await page.waitFor('document.querySelectorAll(".preview-comment textarea").length === 0',timeout,'submitted drafts cleared');
  ok('Due commenti inviati insieme alla stessa sessione nativa con testo, pagina e identità dell’elemento.');
  // A failed send must retain the complete draft.
  await choose({index:1,text:'Commento da conservare'});
  await page.evaluate(`(()=>{const original=window.fetch;window.fetch=(url,options)=>String(url).includes('/messages')?Promise.resolve(new Response(JSON.stringify({error:'Invio rifiutato dal test'}),{status:409,headers:{'Content-Type':'application/json'}})):original(url,options);})()`);
  await page.evaluate('document.querySelector("[data-preview=send]").click()');
  await page.waitFor('document.querySelector(".preview-status").textContent.includes("Invio rifiutato")',timeout,'failed delivery visible');
  assert.equal(await page.evaluate('document.querySelector(".preview-comment textarea").value'),'Commento da conservare');
  ok('Un invio rifiutato conserva i commenti e mostra il motivo.');
  // Reload loses in-memory controller state, but must retain browser drafts.
  await page.send('Page.reload');
  await page.waitFor('!!document.querySelector("[data-conversation-preview]")',timeout,'conversation after reload');
  await page.evaluate('document.querySelector("[data-conversation-preview]").click()');
  await page.waitFor('document.querySelector(".preview-comment textarea")?.value === "Commento da conservare"',timeout,'draft restored after page reload');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'picker after page reload');
  frameID=(await page.send('Page.getFrameTree')).frameTree.childFrames[0].frame.id;
  await until(()=>contexts.has(frameID),'restored preview execution context');
  await page.evaluate('document.querySelector(".preview-comment textarea").focus()');
  await until(async()=>{const value=await evaluateFrame('document.querySelector("[data-archetipo-highlight]").style.display');return value==='block';},'component association restored after reload');
  const restoredColumns=await dimensions();assert.ok(Math.abs(restoredColumns.rail-afterRail.rail)<2);assert.ok(Math.abs(restoredColumns.right-afterRight.right)<2);
  ok('La riapertura della pagina conserva i commenti non inviati e le larghezze scelte.');
  await page.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:true});
  await page.waitFor('document.querySelector("#browser-preview").getBoundingClientRect().width < 400',timeout,'mobile preview');
  assert.equal(await page.evaluate('document.documentElement.scrollWidth > innerWidth'),false);
  const image=await page.send('Page.captureScreenshot',{format:'png'});await fs.writeFile(path.join(dir,'preview-mobile.png'),Buffer.from(image.data,'base64'));
  await evaluateFrame('location.href="/seconda"');
  await until(async()=>{try{return await evaluateFrame('location.pathname')==='/seconda';}catch{return false;}},'navigate preview to another page');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'picker on second page');
  await page.evaluate('document.activeElement.blur();document.querySelector(".preview-comment textarea").focus()');
  await page.waitFor('document.querySelector(".preview-comment-reference").textContent.includes("altra pagina")',timeout,'reference to another page explained');
  assert.equal(await evaluateFrame('document.querySelector("[data-archetipo-highlight]").style.display'),'none');
  await evaluateFrame('location.href="/"');
  await until(async()=>{try{return await evaluateFrame('location.pathname')==='/';}catch{return false;}},'return to commented page');
  await page.waitFor('!document.querySelector("[data-preview=pick]").disabled',timeout,'picker on original page');
  await evaluateFrame('document.querySelector("button").remove()');
  await page.evaluate('document.activeElement.blur();document.querySelector(".preview-comment textarea").focus()');
  await page.waitFor('document.querySelector(".preview-comment-reference").textContent.includes("non è più presente")',timeout,'missing component explained');
  assert.equal(await evaluateFrame('document.querySelector("[data-archetipo-highlight]").style.display'),'none');
  ok('Riferimenti ripristinati dopo reload; nessuna evidenziazione errata su altre pagine o componenti rimossi.');
  await page.evaluate('document.querySelector("[data-preview=stop]").click()');
  await page.waitFor('!document.querySelector("[data-preview=start]").disabled',timeout,'preview stopped');
  const pid=Number(await fs.readFile(path.join(sandbox,'server.pid'),'utf8'));
  await until(()=>{try{process.kill(pid,0);return false;}catch{return true;}},'owned app process terminated');
  ok('Preview e commenti leggibili a 390px; arresto del server realmente verificato sul processo.');
  // Hard viewer death closes the private supervisor pipe and its app tree.
  const started=await fetch(endpoint+'/preview',{method:'POST'});assert.equal(started.status,202);
  await until(async()=>{const state=await (await fetch(endpoint+'/preview')).json();return state.state==='ready';},'second app startup');
  const newPid=Number(await fs.readFile(path.join(sandbox,'server.pid'),'utf8'));assert.notEqual(newPid,pid);
  viewer.child.kill('SIGKILL');
  await until(()=>{try{process.kill(newPid,0);return false;}catch{return true;}},'server cleanup on hard viewer death');
  ok('Anche la terminazione forzata del viewer chiude il server posseduto.');
  assert.deepEqual(exceptions,[],'uncaught browser exceptions');
  await fs.writeFile(path.join(dir,'summary.json'),JSON.stringify({status:'pass',checks},null,2));
  await fs.writeFile(path.join(dir,'report.html'),`<!doctype html><meta charset="utf-8"><h1>Preview smoke: PASS</h1><ul>${checks.map(text=>'<li>'+text+'</li>').join('')}</ul>`);
  console.log('Report: '+dir);
 }catch(error){
  await fs.writeFile(path.join(dir,'failure.json'),JSON.stringify({error:error.stack,received,viewer:viewer?.log(),page:await page?.evaluate('document.body.innerText').catch(()=>null)},null,2));
  throw error;
 }finally{
  await page?.close();await browser?.close();if(viewer?.child.exitCode===null && viewer?.child.signalCode===null)await stopProcess(viewer.child,run);
  control.closeAllConnections();await new Promise(resolve=>control.close(resolve));
 }
}
main().catch(error=>{console.error(error);process.exitCode=1;});
