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
const enterprise=process.argv.includes("--enterprise");
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
 const runDir=await createRunDir(path.join(repoRoot,enterprise?"test/workspaces/enterprise-product-view-smoke":"test/workspaces/chat-first-view-smoke"));
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
  if(enterprise){
   const payload=path.join(runDir,"story.json");
   await fs.writeFile(payload,JSON.stringify({specs:[story("CSV")]}));
   await run("enterprise-story",cli,["spec","add","--file",payload],{cwd:sandbox});
  }
  await fs.writeFile(path.join(sandbox,".gitignore"),".agents/\n");
  const git=async(args,cwd=sandbox)=>(await run("git","git",args,{cwd})).stdout.trim();
  await git(["init","-b","main"]);await git(["config","user.email","tests@example.test"]);await git(["config","user.name","Tests"]);
  await git(["add",".gitignore","hello.txt",".archetipo/config.yaml",".archetipo/shared-runtime.md",".archetipo/backlog.yaml",".archetipo/environment.json",".archetipo/preview.json","scripts/archetipo"]);
  if(enterprise)await git(["add",".archetipo/specs"]);
  await git(["commit","-m","initial"]);
  const baseline=await git(["rev-parse","HEAD"]);
  const bare=path.join(runDir,"origin.git");await fs.mkdir(bare);await git(["init","--bare"],bare);
  await git(["remote","add","origin",bare]);await git(["push","-u","origin","main"]);
  view=await startViewServer(cli,sandbox,env,"/api/board");
  browser=await launchChrome(chrome,path.join(runDir,"chrome"));page=await browser.newPage();
  const exceptions=[];page.on("Runtime.exceptionThrown",e=>exceptions.push(e.exceptionDetails));
  await page.send("Runtime.enable");await page.send("Page.enable");
  await page.send("Page.navigate",{url:view.url});await page.once("Page.loadEventFired",timeout,"page load");
  await page.waitFor(`!!document.querySelector(".conv-work-home .conv-composer-input:not(:disabled)")`,timeout,"chat-first composer");
  await fs.writeFile(path.join(runDir,"home.png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png"})).data,"base64"));
  await page.send("Emulation.setDeviceMetricsOverride",{width:390,height:844,deviceScaleFactor:1,mobile:false});
  assert.ok(await page.evaluate("document.documentElement.scrollWidth<=innerWidth"),"mobile home overflow");
  await fs.writeFile(path.join(runDir,"home-mobile.png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png"})).data,"base64"));
  await page.send("Emulation.clearDeviceMetricsOverride");
  if(enterprise){
   await page.evaluate(`document.querySelector('[data-shell-view="board"]').click()`);
   await page.waitFor(`!!document.querySelector('.card[data-code="US-001"]')`,timeout,"original enterprise story");
   await page.evaluate(`document.querySelector('.card[data-code="US-001"]').click()`);
   await page.waitFor(`document.getElementById("story-view-title")?.textContent.includes("CSV")`,timeout,"story detail");
   await page.evaluate(`document.getElementById("spec-develop-btn").click()`);
  }else{
  await page.evaluate(`(()=>{const input=document.querySelector(".conv-composer-input");input.value="Crea una pagina per esportare i clienti";input.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".conv-composer").requestSubmit();})()`);
  }
  await page.waitFor(`!!document.querySelector(".conv-increment")`,timeout,"work created directly by the first message");
  const index=await (await fetch(view.url+"/api/workspace/conversations")).json();
  const conversationID=index.items?.[0]?.id||index.conversations?.[0]?.id;
  // Obtain the named conversation through its rail, without assuming the
  // listing's envelope shape.
  const id=conversationID||await page.evaluate(`document.querySelector("[data-conversation-id].is-current")?.getAttribute("data-conversation-id") || document.querySelector("[data-conversation-id]")?.getAttribute("data-conversation-id")`);
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

  assert.equal(inc.development_mode,"continuous");
  assert.equal(await page.evaluate(`document.getElementById("workspace-inspector").hidden`),false);
  const proposal={intent:"Problema: condividere i clienti selezionati. Comportamento: esportazione CSV con i soli clienti scelti."};
  const proposed=await cliJSON(["increment","propose",inc.id],proposal);
  assert.equal(proposed.status,"DEVELOPING");
  assert.equal(proposed.revisions[0].approval_source,"request");
  assert.notEqual(proposed.revisions[0].approved,true,"request is not business acceptance");
  assert.equal((await cliJSON(["spec","list"])).items.length,0,"created a synthetic backlog");
  let workflow;
  if(enterprise){
   assert.equal(proposed.linked_stories[0].code,"US-001");
   workflow=await cliJSON(["execution","workflow","start"],{skill:"archetipo-increment",invocation:"/archetipo-increment "+inc.id,scope:inc.id,conversation_id:id,state:{phase:"building",completed:["specification"],remaining:["implementation","verification"]}});
   control.queue.push({kind:"exit",code:17});
   await until(()=>control.received.some(entry=>entry.kind==="turn/start"&&(entry.params?.input||[]).some(input=>input.text?.includes("Resume workflow run "+workflow.id))),"automatic same-session continuation");
   assert.equal((await read()).session.session.native.id,native);
   workflow=await cliJSON(["execution","workflow","show",workflow.id]);
   assert.equal(workflow.recovery_attempts,1);
   assert.deepEqual(workflow.state.completed,["specification"]);
   assert.deepEqual(workflow.state.remaining,["implementation","verification"]);
  }
  await fs.writeFile(path.join(inc.workdir,"hello.txt"),"Clienti in CSV\n");
  await git(["add","hello.txt"],inc.workdir);
  await git(["commit","-m","feat: customer export"],inc.workdir);
  await cliJSON(["increment","ready",inc.id],{summary:"Esportazione pronta",checks:["File CSV verificato"]});
  if(workflow)await cliJSON(["execution","workflow","checkpoint",workflow.id],{revision:workflow.revision,status:"complete",phase:"ready",state:{...workflow.state,phase:"ready"}});
  await finish(1,"La pagina è pronta da provare.");
  await page.waitFor(`!!document.querySelector("#browser-preview iframe")`,timeout,"automatic managed preview");
  await fs.writeFile(path.join(runDir,"work.png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png"})).data,"base64"));
  assert.equal((await read()).session.session.native.id,native);
  assert.equal(await fs.readFile(path.join(sandbox,"hello.txt"),"utf8"),"originale\n");
  await page.evaluate(`document.querySelector('[data-preview="stop"]').click()`);
  await until(async()=>(await (await fetch(convURL+"/preview")).json()).state==="stopped","explicit stop");
  await page.send("Page.reload");await page.once("Page.loadEventFired",timeout,"reload after stop");
  await page.waitFor(`document.querySelector(".preview-status")?.textContent.includes("Premi Preview")`,timeout,"stop survives reload without automatic restart");
  assert.equal((await (await fetch(convURL+"/preview")).json()).state,"stopped");
  await page.evaluate(`document.querySelector('[data-preview="start"]').click()`);
  await page.waitFor(`!!document.querySelector("#browser-preview iframe")`,timeout,"explicit preview restart");
  const publication=await fetch(view.url+"/api/increments/"+inc.id+"/publish",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({revision:1,head_sha:"wrong"})});
  assert.equal(publication.status,409,"stale candidate published");
  await page.waitFor(`!!document.querySelector(".conv-composer-input:not(:disabled)")`,timeout,"feedback composer");
  await page.evaluate(`(()=>{const input=document.querySelector(".conv-composer-input");input.value="Aggiungi anche il formato Excel";input.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".conv-composer").requestSubmit();})()`);
  await until(()=>control.received.filter(e=>e.kind==="turn/start").length>=(enterprise?3:2),"feedback delivered in same session");
  await cliJSON(["increment","feedback",inc.id],{feedback:"Aggiungi Excel"});
  const revised=await cliJSON(["increment","propose",inc.id],{intent:"Esportare CSV ed Excel"});
  assert.equal(revised.status,"DEVELOPING");assert.equal(revised.ready_sha,undefined);
  assert.equal((await cliJSON(["increment","list"],undefined,sandbox)).length,1);
  await cliJSON(["increment","learn",inc.id],{observation:"L’esportazione deve essere leggibile da Excel",decision:"Manteniamo entrambi i formati"});
  if(enterprise){
   await fs.writeFile(path.join(inc.workdir,"hello.txt"),"Clienti in CSV ed Excel\n");
   await git(["add","hello.txt"],inc.workdir);await git(["commit","-m","feat: Excel export"],inc.workdir);
   await cliJSON(["increment","ready",inc.id],{summary:"Esporta CSV ed Excel",checks:["Entrambi i formati verificati"]});
  }
  await finish(2,"Aggiorno l’esportazione nello stesso lavoro.");
  await page.send("Page.reload");await page.once("Page.loadEventFired",timeout,"reload");
  await page.waitFor(`!!document.querySelector(".conv-increment")`,timeout,"durable work after reload");
  assert.equal((await read()).session.session.native.id,native);
  assert.equal((await read()).increment.id,inc.id);
  assert.equal((await read()).increment.learning.length,1);
  assert.equal((await cliJSON(["spec","list"])).items.length,0);
  if(enterprise){
   await page.waitFor(`!!document.querySelector('[data-increment-action="publish"]:not(:disabled)')`,timeout,"tested version awaiting business approval");
   const approved=(await read()).increment.ready_sha;
   await page.evaluate(`document.querySelector('[data-increment-action="publish"]').click()`);
   await until(async()=>(await read()).increment.status==="PROPOSED","one PR after human approval");
   let published=(await read()).increment;
   const repeated=await fetch(view.url+"/api/increments/"+inc.id+"/publish",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({revision:2,head_sha:approved})});
   assert.equal(repeated.status,200);
   const gh=JSON.parse(await fs.readFile(ghState,"utf8"));assert.equal(gh.creates,1);
   const dossier=await (await fetch(view.url+"/api/increments/"+inc.id+"/dossier")).json();
   assert.ok(dossier.markdown.includes("Specification v1")&&dossier.markdown.includes("Specification v2")&&dossier.markdown.includes("US-001")&&dossier.markdown.includes(approved));
   const traceResponse=await fetch(view.url+"/api/spec/US-001");
   const traceDetail=await traceResponse.json();
   assert.equal(traceResponse.status,200,JSON.stringify(traceDetail));
   assert.ok(traceDetail.works.some(work=>work.pr_url?.includes("/pull/128")),JSON.stringify(traceDetail.works));
   await page.evaluate(`document.querySelector('[data-increment-panel="open"]').click()`);
   await page.waitFor(`!!document.querySelector('[data-linked-story="US-001"]')`,timeout,"work links original story");
   await page.evaluate(`document.querySelector('[data-linked-story="US-001"]').click()`);
   await page.waitFor(`document.getElementById("story-linked-work")?.textContent.includes("Apri PR")`,timeout,"story links work and PR",()=>page.evaluate(`JSON.stringify({status:document.getElementById("story-status")?.textContent,works:document.getElementById("story-linked-work")?.innerHTML,scripts:!!window.StoryTrace,pane:document.getElementById("workspace-shell").dataset.view})`));
   assert.ok(await page.evaluate(`document.getElementById("story-linked-work").textContent.includes("Apri PR")`));
   await fs.writeFile(path.join(runDir,"story.png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png"})).data,"base64"));
   await page.send("Emulation.setDeviceMetricsOverride",{width:390,height:844,deviceScaleFactor:1,mobile:false});
   assert.ok(await page.evaluate("document.documentElement.scrollWidth<=innerWidth"),"story detail mobile overflow");
   await fs.writeFile(path.join(runDir,"story-mobile.png"),Buffer.from((await page.send("Page.captureScreenshot",{format:"png"})).data,"base64"));
   await page.send("Emulation.clearDeviceMetricsOverride");

   const original=await cliJSON(["spec","show","US-001"],undefined,sandbox);
   assert.equal(original.spec.status,"TODO");
   gh.pr.state="MERGED";gh.pr.headRefOid="developer-revision";await fs.writeFile(ghState,JSON.stringify(gh));
   published=await cliJSON(["increment","refresh",inc.id]);
   assert.equal(published.status,"DELIVERED");assert.equal(published.approved_sha,approved);assert.equal(published.integration_sha,"developer-revision");
   assert.equal((await cliJSON(["spec","list"],undefined,sandbox)).items.length,1);
   ok("Storia originale ↔ lavoro ↔ PR; dossier esportabile; recupero dopo crash; approvazione umana e un’unica PR; merge dei developer senza falsare il backlog.");
  }
  assert.deepEqual(exceptions,[],"uncaught browser exceptions");
  ok("Primo messaggio → worktree e sessione; specifica senza conferma aggiuntiva; preview automatica; feedback e reload nello stesso lavoro; PR protetta.");
 } catch(error) {failure=error;console.error(error.stack || error);}
 finally {
  if(browser)await browser.close();
  if(view)await stopProcess(view.child,run);
  if(incrementRoot)await run("cleanup",cli,["execution","environment","cleanup"],{cwd:incrementRoot,allowFailure:true});
  await control.close();
  await fs.writeFile(path.join(runDir,"summary.json"),JSON.stringify({ok:!failure,checks,error:failure?.stack},null,2));
  console.log("Report: "+runDir);
 }
 if(failure)process.exitCode=1;
}
main().catch(error=>{console.error(error);process.exitCode=1;});
