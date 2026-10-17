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
 const run=makeRunCommand(env);let viewer,contender,browser,page;
 try{
  await buildCLI(cli,repo,run);
  await run('init',cli,['init','--tool','codex','--connector','file','--yes'],{cwd:sandbox});
  await fs.appendFile(path.join(sandbox,'.archetipo/config.yaml'),'\nexecution:\n  default_provider:\n    id: codex\n    config:\n      command: '+JSON.stringify(path.join(repo,'test/e2e/support/fake-codex.mjs'))+'\n      model: gpt-fake\n      sandbox: workspace-write\n');
  const port=await freePort();
  const app=path.join(sandbox,'server.mjs');
  await fs.writeFile(app,`import http from 'node:http';import fs from 'node:fs';fs.writeFileSync('server.pid',String(process.pid));for(let i=0;i<150;i++)console.log('Output fixture '+i);http.createServer((req,res)=>{res.setHeader('Content-Type','text/html');res.end('<!doctype html><title>Preview smoke</title><h1>Risultato</h1><p id=descrizione>Descrizione del risultato</p><a href="/seconda">Seconda pagina</a><button data-testid="salva" onclick="document.body.dataset.clicked=1">Salva</button><script>window.mountId=Math.random();</script>');}).listen(${port},'127.0.0.1');`);
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
  await page.evaluate(`document.querySelector('[data-inspector-tab="details"]').click()`);
  await page.waitFor('!document.querySelector("#inspector-details").hidden && !!document.querySelector(".conv-increment-panel")',timeout,'increment in shared inspector');
  await page.evaluate(`document.querySelector('[data-inspector-tab="preview"]').click()`);
  await page.waitFor('!document.querySelector("#browser-preview").hidden',timeout,'return to mounted preview');
  assert.equal(await evaluateFrame('window.mountId'),mount,'changing inspector tabs remounted preview');
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
  await fs.writeFile(path.join(dir,'failure.json'),JSON.stringify({error:error.stack,received,viewer:viewer?.log(),page:await page?.evaluate('document.body.innerText').catch(()=>null),focus:await page?.evaluate('({active:document.activeElement.outerHTML,editors:Array.from(document.querySelectorAll(".preview-comment-editor")).map(e=>e.hidden)})').catch(()=>null)},null,2));
  throw error;
 }finally{
  await page?.close();await browser?.close();if(viewer?.child.exitCode===null && viewer?.child.signalCode===null)await stopProcess(viewer.child,run);
  if(contender?.child.exitCode===null && contender?.child.signalCode===null)await stopProcess(contender.child,run);
  control.closeAllConnections();await new Promise(resolve=>control.close(resolve));
 }
}
main().catch(error=>{console.error(error);process.exitCode=1;});
