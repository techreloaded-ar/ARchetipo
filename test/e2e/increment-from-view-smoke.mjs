#!/usr/bin/env node
// Real View, CLI, connector, native adapter and browser. Only the agent and
// GitHub are simulated. No credentials or non-local traffic are used.
import fs from "node:fs/promises";
import path from "node:path";
import http from "node:http";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import { findChrome, launchChrome } from "./viewer-page-load-smoke.mjs";
import { buildCLI, createRunDir, makeRunCommand, startViewServer, stopProcess, readBody } from "./support/view-smoke-harness.mjs";

const repoRoot=path.resolve(path.dirname(fileURLToPath(import.meta.url)),"../..");
const cli=path.join(repoRoot,"test/e2e/.bin/archetipo");
const fakeCodex=path.join(repoRoot,"test/e2e/support/fake-codex.mjs");
const checks=[];
const ok=text=>{checks.push(text);console.log("✓ "+text);};
const timeout=25000;
async function until(fn,label){
 const end=Date.now()+timeout;
 while(Date.now()<end){if(await fn())return;await delay(40);}
 throw new Error("Timeout: "+label);
}
async function controlServer(){
 const queue=[],received=[];
 const server=http.createServer(async(req,res)=>{
  res.setHeader("Content-Type","application/json");
  if(new URL(req.url,"http://localhost").pathname==="/next"){res.end(JSON.stringify(queue.shift()||{kind:"none"}));return;}
  if(req.url==="/received"){received.push(JSON.parse(await readBody(req)));res.end("{}");return;}
  res.writeHead(404);res.end("{}");
 });
 await new Promise(resolve=>server.listen(0,"127.0.0.1",resolve));
 return {queue,received,url:"http://127.0.0.1:"+server.address().port,close:()=>new Promise(resolve=>server.close(resolve))};
}
async function fakeGhMain(){
 const fs=await import("node:fs/promises");
 const {execFileSync}=await import("node:child_process");
 const statePath=process.env.FAKE_GH_STATE;
 let state;
 try{state=JSON.parse(await fs.readFile(statePath,"utf8"));}catch{state={pr:null,creates:0};}
 const args=process.argv.slice(2);
 if(args[0]!=="pr")throw new Error("Unexpected gh command");
 if(args[1]==="create"){
  if(state.pr)throw new Error("Duplicate PR creation");
  state.creates++;
  state.pr={number:128,url:"https://github.com/example/product/pull/128",state:"OPEN",headRefOid:execFileSync("git",["rev-parse","HEAD"],{encoding:"utf8"}).trim()};
  await fs.writeFile(statePath,JSON.stringify(state));
  console.log(state.pr.url);
 }else if(args[1]==="list")console.log(JSON.stringify(state.pr?[state.pr]:[]));
 else if(args[1]==="view")console.log(JSON.stringify(state.pr));
 else throw new Error("Unexpected gh operation");
}
const story=(format)=>({code:"US-001",title:"Esportare i clienti in "+format,epic:{code:"EP-001",title:"Esportazione clienti"},priority:"MEDIUM",points:3,status:"TODO",scope:"MVP",blocked_by:[],body:"## User story\nCome responsabile commerciale voglio esportare i clienti.\n## Dimostrazione\nApro un file "+format+" con i clienti.\n## Criteri\n- [ ] Il file contiene solo i clienti attesi."});
const taskBody="## Objective\nImplement the agreed result.\n## Context\nhello.txt\n## Allowed changes\nhello.txt\n## Steps\n1. Apply the accepted change.\n## Verification\nRead the resulting file.\n## Done\nThe expected content is present.\n## Blockers\nNone.";

async function main(){
 if(process.platform==="win32"){console.log("SKIP: fake agent uses a POSIX shebang");return;}
 const chrome=await findChrome(process.env.ARCHETIPO_CHROME);
 if(!chrome)throw new Error("Chrome required: set ARCHETIPO_CHROME");
 const runDir=await createRunDir(path.join(repoRoot,"test/workspaces/increment-from-view-smoke"));
 const sandbox=path.join(runDir,"sandbox"),bin=path.join(runDir,"bin"),ghState=path.join(runDir,"github.json");
 await fs.mkdir(sandbox,{recursive:true});await fs.mkdir(bin,{recursive:true});
 const control=await controlServer();
 const env={...process.env,ARCHETIPO_DATA_DIR:repoRoot,ARCHETIPO_STATE_DIR:path.join(runDir,"state"),FAKE_CODEX_CONTROL:control.url,FAKE_GH_STATE:ghState,PATH:bin+path.delimiter+process.env.PATH};
 const run=makeRunCommand(env);
 await fs.writeFile(path.join(bin,"gh"),"#!/usr/bin/env node\n("+fakeGhMain.toString()+")().catch(e=>{console.error(e.message);process.exit(1)});\n",{mode:0o755});
 let view,browser,page,failure,incrementRoot;
 try{
  await buildCLI(cli,repoRoot,run);
  await run("init",cli,["init","--tool","codex","--connector","file","--yes"],{cwd:sandbox});
  await fs.appendFile(path.join(sandbox,".archetipo/config.yaml"),"\nexecution:\n  default_provider:\n    id: codex\n    config:\n      command: "+JSON.stringify(fakeCodex)+"\n      model: gpt-fake\n      sandbox: workspace-write\n");
  await fs.writeFile(path.join(sandbox,"hello.txt"),"originale\n");
  await fs.mkdir(path.join(sandbox,"scripts/archetipo"),{recursive:true});
  await fs.copyFile(path.join(repoRoot,"test/e2e/support/managed-resource.mjs"),path.join(sandbox,"scripts/archetipo/managed-resource.mjs"));
  const resourceCommand=action=>({name:action,argv:[process.execPath,"scripts/archetipo/managed-resource.mjs",action],timeout_seconds:10});
  const environmentInput=path.join(runDir,"environment.json");
  await fs.writeFile(environmentInput,JSON.stringify({version:1,ports:["DATABASE"],resources:[{name:"database",start:resourceCommand("start"),probe:resourceCommand("probe"),stop:resourceCommand("stop"),stopped:resourceCommand("stopped")}],probe:[{name:"ready",argv:[process.execPath,"--version"],timeout_seconds:10}],checks:{}}));
  await run("environment-configure",cli,["execution","environment","configure","--file",environmentInput],{cwd:sandbox});
  await fs.writeFile(path.join(sandbox,"scripts/archetipo/preview.mjs"),'import http from "node:http"; http.createServer((req,res)=>res.end("Preview fixture")).listen(Number(process.argv[2]),"127.0.0.1");\n');
  const previewInput=path.join(runDir,"preview.json");
  await fs.writeFile(previewInput,JSON.stringify({version:1,directory:".",argv:[process.execPath,"scripts/archetipo/preview.mjs","{port}"],url:"http://127.0.0.1:{port}",prepare_environment:true}));
  await run("preview-configure",cli,["execution","preview","configure","--file",previewInput],{cwd:sandbox});
  await run("environment-publish",cli,["execution","environment","publish","--asset","scripts/archetipo/preview.mjs","--asset","scripts/archetipo/managed-resource.mjs"],{cwd:sandbox});
  await fs.writeFile(path.join(sandbox,".gitignore"),".agents/\n");
  const git=async(args,cwd=sandbox)=>(await run("git","git",args,{cwd})).stdout.trim();
  await git(["init","-b","main"]);await git(["config","user.email","tests@example.test"]);await git(["config","user.name","Tests"]);
  await git(["add",".gitignore","hello.txt",".archetipo/config.yaml",".archetipo/shared-runtime.md",".archetipo/backlog.yaml"]);
  await git(["commit","-m","initial"]);
  const baseline=await git(["rev-parse","HEAD"]);
  const bare=path.join(runDir,"origin.git");await fs.mkdir(bare);await git(["init","--bare"],bare);
  await git(["remote","add","origin",bare]);await git(["push","-u","origin","main"]);
  view=await startViewServer(cli,sandbox,env,"/api/board");
  browser=await launchChrome(chrome,path.join(runDir,"chrome"));page=await browser.newPage();
  const exceptions=[];page.on("Runtime.exceptionThrown",e=>exceptions.push(e.exceptionDetails));
  await page.send("Runtime.enable");await page.send("Page.enable");
  await page.send("Page.navigate",{url:view.url});await page.once("Page.loadEventFired",timeout,"page load");
  await page.waitFor('!!document.querySelector(\'[data-conversation-activity="increment"]:not(:disabled)\')',timeout,"increment activity");
  await page.evaluate('document.querySelector(\'[data-conversation-activity="increment"]\').click()');
  await page.evaluate('(()=>{const input=document.querySelector(".conv-composer-input");input.value="Vorrei esportare i clienti per la revisione commerciale";input.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".conv-composer").requestSubmit();})()');
  await page.waitFor('!!document.querySelector(".conv-increment")',timeout,"increment in the existing conversation UI");
  const index=await (await fetch(view.url+"/api/workspace/conversations")).json();
  const conversationID=index.items?.[0]?.id||index.conversations?.[0]?.id;
  // Obtain the named conversation through its rail, without assuming the
  // listing's envelope shape.
  const id=conversationID||await page.evaluate('document.querySelector("[data-conversation-id].is-current")?.getAttribute("data-conversation-id") || document.querySelector("[data-conversation-id]")?.getAttribute("data-conversation-id")');
  assert.ok(id,"conversation id");
  let convURL=view.url+"/api/workspace/conversations/"+id;
  const read=async()=>await (await fetch(convURL)).json();
  let current=await read(),inc=current.increment;incrementRoot=inc.workdir;
  assert.ok(inc&&inc.workdir!==sandbox);assert.equal(current.session.session.environment.working_dir,inc.workdir);
  const native=current.session.session.native.id;
  const cliJSON=async(args,payload,cwd=inc.workdir)=>{
   // makeRunCommand supports input only through its separate option; use a
   // real payload file so the same invocation works on all supported hosts.
   const filename=path.join(runDir,"payload-"+Date.now()+"-"+Math.random().toString(16).slice(2)+".json");
   if(payload!==undefined){await fs.writeFile(filename,JSON.stringify(payload));args=[...args,"--file",filename];}
   return JSON.parse((await run("CLI",cli,args,{cwd})).stdout).data;
  };
  const finish=async(n,text)=>{
   const nativeTurn=(await read()).session.current_turn.native_id;
   control.queue.push({kind:"emit",method:"item/agentMessage/delta",params:{turnId:nativeTurn,itemId:"answer-"+n,delta:text}});
   control.queue.push({kind:"emit",method:"item/completed",params:{turnId:nativeTurn,item:{type:"agentMessage",id:"answer-"+n,text}}});
   control.queue.push({kind:"emit",method:"turn/completed",params:{turn:{id:nativeTurn,status:"completed"}}});
   await until(async()=>!(await read()).increment.busy,"idle native turn");
  };
  const openDetails=async()=>{
   await page.waitFor('!!document.querySelector("[data-increment-panel=open]")',timeout,"increment details entry");
   if(!await page.evaluate('!!document.querySelector(".conv-increment-panel")')) await page.evaluate('document.querySelector("[data-increment-panel=open]").click()');
   await page.waitFor('!!document.querySelector(".conv-increment-panel")',timeout,"increment details panel");
  };
  const closeDetails=async()=>{await page.evaluate('document.querySelector("[data-increment-panel=close]")?.click()');};
  const click=async(action)=>{await openDetails();await page.waitFor('!!document.querySelector(\'[data-increment-action="'+action+'"]:not(:disabled)\')',timeout,action);await page.evaluate('document.querySelector(\'[data-increment-action="'+action+'"]\').click()');await closeDetails();};
  const propose=async(format,n)=>{
   await cliJSON(["increment","propose",inc.id],{intent:"Preparare la revisione commerciale con un file "+format+".\n\n"+Array.from({length:24},(_,i)=>"### Punto "+(i+1)+"\nDescrizione concordata: esportare i clienti selezionati e verificare il contenuto del file prima della revisione commerciale.").join("\n\n")+"\n\nFine dell’intento da leggere.",stories:[story(format)]});
   await finish(n,"Ho preparato l’intento e una storia. Leggili prima di approvare.\n\n"+Array.from({length:80},(_,i)=>"Passaggio "+(i+1)+": abbiamo chiarito il risultato e le verifiche da eseguire prima dell’approvazione.").join("\n\n"));
   await openDetails();
   await page.waitFor('document.querySelector(".conv-increment-panel")?.textContent.includes("'+format+'")',timeout,"persisted intent");
  };
  let recoveredTransport=false;
  const develop=async(format,n)=>{
   const legacyTasks=!recoveredTransport;
   const current=await cliJSON(["increment","show",inc.id]);
   let workflow=await cliJSON(["execution","workflow","start"],{skill:"archetipo-increment",invocation:"/archetipo-increment "+inc.id,scope:inc.id,conversation_id:id,state:{phase:"implementation",revision:current.revisions.length,stories:["US-001"],review_attempts:0,recoveries:{}}});
   if(legacyTasks) await cliJSON(["spec","plan","US-001"],{plan_body:"## Baseline\nhello.txt contains the previous result.\n## Solution\nChange the output and verify it.",tasks:[{id:"TASK-01",title:"Implement",type:"Impl",status:"TODO",dependencies:[],body:taskBody},{id:"TASK-02",title:"Verify",type:"Test",status:"TODO",dependencies:["TASK-01"],body:taskBody}]});
   if(legacyTasks) await cliJSON(["spec","start","US-001"]);
   else await cliJSON(["spec","move","US-001","--to","in_progress"]);
   await fs.writeFile(path.join(inc.workdir,"hello.txt"),"Clienti in "+format+"\n");
   if(legacyTasks) await cliJSON(["task","done","US-001","TASK-01"]);
   if(!recoveredTransport){
    control.queue.push({kind:"exit",code:17});
    await until(()=>control.received.some(entry=>entry.kind==="turn/start"&&(entry.params?.input||[]).some(input=>input.text?.includes("Resume workflow run "+workflow.id))),"automatic same-session workflow continuation");
    const resumed=await read();
    assert.equal(resumed.session.session.native.id,native);
    assert.equal(resumed.increment.id,inc.id);
    const persisted=await cliJSON(["spec","show","US-001"]);
    assert.equal(persisted.tasks.find(task=>task.id==="TASK-01").status,"DONE");
    assert.equal(persisted.tasks.find(task=>task.id==="TASK-02").status,"TODO");
    workflow=await cliJSON(["execution","workflow","show",workflow.id]);
    assert.equal(workflow.recovery_attempts,1);
    recoveredTransport=true;
    ok("Una caduta del processo agente riprende automaticamente la stessa sessione, conservando task e approvazione.");
   }
   if(legacyTasks) await cliJSON(["task","done","US-001","TASK-02"]);
   await cliJSON(["spec","review","US-001"]);
   await cliJSON(["spec","move","US-001","--to","done"]);
   if(!legacyTasks){
    const planless=await cliJSON(["spec","show","US-001"]);
    assert.equal(planless.tasks.length,0);
    assert.ok(!planless.plan_body,"feedback revision should have no stale plan");
   }
   await cliJSON(["increment","ready",inc.id],{summary:"Apri hello.txt: il risultato concordato è Clienti in "+format+".",checks:["Contenuto del file verificato: Clienti in "+format]});
   await cliJSON(["execution","workflow","checkpoint",workflow.id],{revision:workflow.revision,status:"complete",phase:"ready",state:{...workflow.state,phase:"ready"}});
   if(!legacyTasks) ok("La revisione successiva arriva a REVIEW senza piano o task sintetici, mantenendo approvazioni e pubblicazione unica.");
   await finish(n,"Il risultato è pronto. Provalo e dimmi cosa cambiare.");
   await openDetails();
   await page.waitFor('!!document.querySelector(\'[data-increment-action="publish"]:not(:disabled)\')',timeout,"business review");
   await closeDetails();
  };
  await propose("CSV",1);
  const collapsedIntent=await page.evaluate('(()=>{const summary=document.querySelector(\'[data-increment-section="intent"]>summary\'),r=summary.getBoundingClientRect(),card=summary.closest(".conv-increment-panel");const button=card.querySelector(\'[data-increment-action="approve-intent"]\'),b=button.getBoundingClientRect();return {visible:document.elementFromPoint(r.left+r.width/2,r.top+r.height/2)===summary&&document.elementFromPoint(b.left+b.width/2,b.top+b.height/2)===button,cardHeight:card.clientHeight,content:card.scrollHeight};})()');
  assert.equal(collapsedIntent.visible,true,"intent revision remains visible beside a long conversation: "+JSON.stringify(collapsedIntent));
  const verifyIntentScroll=async(width,height)=>{
   await page.send("Emulation.setDeviceMetricsOverride",{width,height,deviceScaleFactor:1,mobile:false});
   await page.evaluate('(()=>{const section=document.querySelector(\'[data-increment-section="intent"]\');if(!section.open)section.querySelector("summary").click();})()');
   await page.waitFor('document.querySelector(\'[data-increment-section="intent"]\')?.open',timeout,"expanded long intent");
   const geometry=await page.evaluate('(()=>{const card=document.querySelector(".conv-increment-body"),r=card.getBoundingClientRect();return {height:card.clientHeight,content:card.scrollHeight,overflow:getComputedStyle(card).overflowY,x:r.left+r.width/2,y:r.top+Math.min(100,r.height/2)};})()');
   assert.ok(["auto","scroll"].includes(geometry.overflow)&&geometry.content>geometry.height,"long increment needs a bounded scrolling area: "+JSON.stringify(geometry));
   // Hit testing must see the expanded panel, not the compositor frame
   // from before the viewport/details changed. Wait for paint, not a timer.
   await page.send("Input.dispatchMouseEvent",{type:"mouseMoved",x:geometry.x,y:geometry.y});
   await page.evaluate("new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))");
   await page.send("Input.dispatchMouseEvent",{type:"mouseWheel",x:geometry.x,y:geometry.y,deltaX:0,deltaY:100000});
   await page.waitFor('document.querySelector(".conv-increment-body")?.scrollTop>100',timeout,"wheel scroll through the long intent");
   await page.waitFor('(()=>{const card=document.querySelector(".conv-increment-body");return card.scrollHeight-card.clientHeight-card.scrollTop<2;})()',timeout,"end of increment content");
   const reachable=await page.evaluate('(()=>{const button=document.querySelector(\'[data-increment-action="approve-intent"]\'),composer=document.querySelector(".conv-composer-input"),r=button.getBoundingClientRect(),c=composer.getBoundingClientRect();return {button:document.elementFromPoint(r.left+r.width/2,r.top+r.height/2)?.closest("[data-increment-action]")===button,composer:c.top>=0&&c.bottom<=innerHeight};})()');
   assert.equal(reachable.button,true,"approval remains reachable after scrolling");
   if(width>1100) assert.equal(reachable.composer,true,"desktop composer remains reachable");
   await page.evaluate('document.querySelector(".conv-increment-body").scrollTop -= 300');
   await page.waitFor('(()=>{const c=document.querySelector(".conv-increment-body");return c.scrollHeight-c.clientHeight-c.scrollTop>100;})()',timeout,"reading inside the intent");
   const previousTop=await page.evaluate('(()=>{const c=document.querySelector(".conv-increment-body");c.focus({preventScroll:true});window.incrementReadingBody=c;return c.scrollTop;})()');
   await page.waitFor('window.incrementReadingBody && !window.incrementReadingBody.isConnected',timeout,"polling with increment details open");
   assert.ok(Math.abs(await page.evaluate('document.querySelector(".conv-increment-body").scrollTop')-previousTop)<2,"redrawing the conversation preserves the reading position: "+JSON.stringify({width,previousTop,current:await page.evaluate('(()=>{const c=document.querySelector(".conv-increment-body");return {top:c.scrollTop,height:c.clientHeight,content:c.scrollHeight,open:document.querySelector("[data-increment-section=intent]").open};})()')}));
   assert.equal(await page.evaluate('document.activeElement===document.querySelector(".conv-increment-body")'),true,"keyboard focus survives redraw");
   await fs.writeFile(path.join(runDir,"long-intent-"+width+".png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png"})).data,"base64"));
  };
  await verifyIntentScroll(1280,800);
  await verifyIntentScroll(390,844);
  await closeDetails();
  await page.send("Emulation.clearDeviceMetricsOverride");
  ok("Il laterale scorre su desktop e mobile, conserva lettura e focus al polling e lascia spazio al compositore quando chiuso.");
  const parentBefore=await cliJSON(["spec","list"],undefined,sandbox);assert.equal(parentBefore.items.length,0);
  const previewURL=convURL+"/preview";
  assert.equal((await fetch(previewURL,{method:"POST"})).status,202);
  await until(async()=>(await (await fetch(previewURL)).json()).state==="ready","discovery preview owns the environment");
  const blockedResult=await run("blocked-prepare",cli,["execution","environment","prepare"],{cwd:inc.workdir,allowFailure:true});
  const blocked=JSON.parse(blockedResult.stdout||blockedResult.stderr);
  assert.equal(blocked.error?.code,"E_CONFLICT","preview must hold the standalone environment lifetime");
  await click("approve-intent");
  await until(()=>control.received.filter(e=>e.kind==="turn/start").length>=2,"development turn");
  const releasedPreview=await (await fetch(previewURL)).json();
  assert.equal(releasedPreview.url,undefined,"development must release the preview proxy");
  assert.ok(releasedPreview.resources.resources.every(resource=>resource.state==="ready"),"development must prepare its own resource generation");
  assert.equal((await read()).session.session.native.id,native);
  ok("Avviare lo sviluppo arresta la preview posseduta nella sandbox e prepara l’ambiente nella stessa sessione nativa.");
  // A planning worker can finish before an implementation worker asks for
  // permission. Both belong to the still-active parent native turn.
  control.queue.push({kind:"emit",method:"turn/started",params:{threadId:"child-plan",turn:{id:"child-plan-turn"}}});
  control.queue.push({kind:"emit",method:"turn/completed",params:{threadId:"child-plan",turn:{id:"child-plan-turn",rootTurnId:"turn-2",status:"completed"}}});
  control.queue.push({kind:"request",id:0,method:"item/commandExecution/requestApproval",params:{threadId:"child-implement",turnId:"child-implement-turn",reason:"Consenti la verifica locale della fixture?",availableDecisions:["accept","cancel"]}});
  await page.waitFor('!!document.querySelector(\'[data-run-approval-id="0"][data-run-option-id="allow"]:not(:disabled)\')',timeout,"delegated approval after planning completes");
  assert.equal((await read()).session.current_turn.state,"ACTIVE");
  await page.evaluate('document.querySelector(\'[data-run-approval-id="0"][data-run-option-id="allow"]\').click()');
  await until(()=>control.received.some(e=>e.kind==="response"&&e.id===0&&e.result?.decision==="accept"),"native approval response");
  await until(async()=>{const view=await read();return view.session.pending_approvals.length===0&&view.session.work==="TURN_ACTIVE";},"approval resolved with parent still active");
  assert.equal(control.received.filter(e=>e.kind==="response"&&e.id===0).length,1);
  control.queue.push({kind:"emit",method:"turn/completed",params:{threadId:"child-implement",turn:{id:"child-implement-turn",status:"completed"}}});
  ok("Il consenso al worker delegato viene consegnato una volta senza chiudere il turno principale.");
  await develop("CSV",2);
  assert.equal(await fs.readFile(path.join(sandbox,"hello.txt"),"utf8"),"originale\n");
  assert.equal(await git(["rev-parse","HEAD"]),baseline);
  ok("Intento approvato e sviluppo restano nel worktree e nel backlog interno.");
  const reviewedSHA=(await read()).increment.ready_sha;
  const beforeQuestion=(await read()).session.current_turn.native_id;
  await page.evaluate('(()=>{const input=document.querySelector(".conv-composer-input");input.value="Come posso verificare il risultato?";input.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".conv-composer").requestSubmit();})()');
  await until(async()=>{const v=await read();return v.session.current_turn?.native_id!==beforeQuestion&&v.session.work==="TURN_ACTIVE";},"question about the result");
  await finish(3,"Apri il file descritto nelle istruzioni di prova.");
  assert.equal((await read()).increment.status,"REVIEW");
  assert.equal((await read()).increment.ready_sha,reviewedSHA);
  ok("Una domanda sul risultato non diventa una richiesta di modifica.");
  const beforeFeedback=(await read()).session.current_turn.native_id;
  await page.evaluate('(()=>{const input=document.querySelector(".conv-composer-input");input.value="Preferisco Excel al CSV";input.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".conv-composer").requestSubmit();})()');
  await until(async()=>{const v=await read();return v.session.current_turn?.native_id!==beforeFeedback&&v.session.work==="TURN_ACTIVE";},"feedback interview");
  await cliJSON(["increment","feedback",inc.id],{feedback:"Preferisco Excel al CSV"});
  await propose("Excel",4);
  await stopProcess(view.child,run);
  view=await startViewServer(cli,sandbox,env,"/api/board");
  convURL=view.url+"/api/workspace/conversations/"+id;
  const resumesBefore=control.received.filter(e=>e.kind==="thread/resume").length;
  await page.send("Page.navigate",{url:view.url});await page.once("Page.loadEventFired",timeout,"viewer after restart");
  await page.waitFor('!!document.querySelector(\'[data-conversation-id="'+id+'"]\')',timeout,"existing conversation after restart");
  await page.evaluate('document.querySelector(\'[data-conversation-id="'+id+'"]\').click()');
  await page.waitFor('!!document.querySelector(".conv-increment")',timeout,"same increment after restart");
  assert.equal(control.received.filter(e=>e.kind==="thread/resume").length,resumesBefore,"read-only history does not resume the native runtime");
  await click("approve-intent");
  await until(()=>control.received.filter(e=>e.kind==="thread/resume").length===resumesBefore+1,"native resume before the increment action");
  assert.equal((await read()).session.session.native.id,native);
  assert.equal(control.received.filter(e=>e.kind==="thread/start").length,1,"no replacement native thread");
  ok("Dopo il riavvio l’azione riprende la stessa sessione nativa; leggere la cronologia non avvia l’agente.");
  await until(()=>control.received.filter(e=>e.kind==="turn/start").length>=5,"second development");
  await develop("Excel",5);
  current=await read();assert.equal(current.session.session.native.id,native);assert.equal(current.increment.id,inc.id);assert.equal(current.increment.revisions.length,2);
  const localStories=await cliJSON(["spec","list"]);assert.equal(localStories.items.length,1);assert.equal(localStories.items[0].code,"US-001");
  ok("Il feedback aggiorna la stessa storia e conserva sessione, sandbox e revisioni.");
  await openDetails();
  await page.evaluate('document.querySelector(\'[data-increment-section="intent"]\').open=true');
  await page.send("Page.reload");await page.once("Page.loadEventFired",timeout,"reload");
  await page.waitFor('!!document.querySelector(".conv-increment")',timeout,"increment after reload");
  await fs.writeFile(path.join(runDir,"desktop.png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png",captureBeyondViewport:true})).data,"base64"));
  await page.send("Emulation.setDeviceMetricsOverride",{width:390,height:844,deviceScaleFactor:1,mobile:false});
  assert.ok(await page.evaluate("document.documentElement.scrollWidth<=document.documentElement.clientWidth"),"mobile overflow");
  await fs.writeFile(path.join(runDir,"mobile.png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png"})).data,"base64"));
  await page.send("Emulation.clearDeviceMetricsOverride");
  await click("publish");
  await until(async()=>(await read()).increment.status==="PROPOSED","PR publication");
  const proposed=await cliJSON(["spec","list"],undefined,sandbox);assert.equal(proposed.items.length,1);assert.equal(proposed.items[0].status,"TODO");assert.match(proposed.items[0].body,/Proposta di incremento/);
  const frozen=(await read()).increment;
  await cliJSON(["increment","publish",inc.id],{revision:2,head_sha:frozen.ready_sha},sandbox);
  let hosting=JSON.parse(await fs.readFile(ghState,"utf8"));assert.equal(hosting.creates,1);
  assert.equal((await cliJSON(["spec","list"],undefined,sandbox)).items.length,1);
  ok("La PR pubblica solo la storia finale come proposta; un retry non duplica PR o storie.");
  hosting.pr.state="MERGED";await fs.writeFile(ghState,JSON.stringify(hosting));
  await click("refresh");
  await until(async()=>(await read()).increment.status==="DELIVERED","merge reconciliation");
  assert.equal((await cliJSON(["spec","list"],undefined,sandbox)).items[0].status,"DONE");
  assert.equal(await git(["rev-parse","HEAD"]),baseline);
  ok("Le storie sono consegnate solo dopo il merge confermato; il viewer non esegue il merge.");
  assert.deepEqual(exceptions,[]);
  await page.waitFor('!!document.querySelector(".conv-composer button[type=submit]:not(:disabled)")',timeout,"conversation available after reconciliation");
  await page.evaluate('document.querySelector("[data-conversation-new]").click()');
  await page.waitFor('!!document.querySelector(\'[data-conversation-activity="conversation"]:not(:disabled), [data-conversation-open]:not(:disabled)\')',timeout,"free conversation entry");
  await page.evaluate('document.querySelector(\'[data-conversation-activity="conversation"]:not(:disabled), [data-conversation-open]:not(:disabled)\').click()');
  await page.waitFor('!!document.querySelector(".conv-composer-input:not(:disabled)")',timeout,"free conversation opened");
  assert.equal(await page.evaluate('document.querySelector(".conv-increment")===null'),true);
  ok("Nuova conversazione libera resta disponibile separatamente dall’incremento.");
 }catch(error){failure=error;
  if(page){
   try{
    await fs.writeFile(path.join(runDir,"failure.png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png"})).data,"base64"));
    await fs.writeFile(path.join(runDir,"failure.txt"),await page.evaluate("document.body.innerText"));
   }catch{}
  }
 }
 finally{
  await page?.close();await browser?.close();
  if(view)await stopProcess(view.child,run);
  if(incrementRoot)await run("environment-cleanup",cli,["execution","environment","cleanup"],{cwd:incrementRoot,allowFailure:true});
  await control.close();
  await fs.writeFile(path.join(runDir,"summary.json"),JSON.stringify({ok:!failure,checks,error:failure?.stack},null,2));
  await fs.writeFile(path.join(runDir,"report.html"),"<meta charset=utf-8><title>Increment smoke</title><h1>"+(failure?"FAIL":"PASS")+"</h1><pre>"+checks.map(s=>s.replaceAll("&","&amp;").replaceAll("<","&lt;")).join("\n")+"</pre>");
 }
 console.log("Report: "+runDir);
 if(failure)throw failure;
}
main().catch(error=>{console.error(error.stack);process.exitCode=1;});
