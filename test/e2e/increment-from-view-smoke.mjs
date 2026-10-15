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
  if(req.url==="/next"){res.end(JSON.stringify(queue.shift()||{kind:"none"}));return;}
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
 let view,browser,page,failure;
 try{
  await buildCLI(cli,repoRoot,run);
  await run("init",cli,["init","--tool","codex","--connector","file","--yes"],{cwd:sandbox});
  await fs.appendFile(path.join(sandbox,".archetipo/config.yaml"),"\nexecution:\n  default_provider:\n    id: codex\n    config:\n      command: "+JSON.stringify(fakeCodex)+"\n      model: gpt-fake\n      sandbox: workspace-write\n");
  await fs.writeFile(path.join(sandbox,"hello.txt"),"originale\n");
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
  const convURL=view.url+"/api/workspace/conversations/"+id;
  const read=async()=>await (await fetch(convURL)).json();
  let current=await read(),inc=current.increment;
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
   control.queue.push({kind:"emit",method:"item/agentMessage/delta",params:{turnId:"turn-"+n,itemId:"answer-"+n,delta:text}});
   control.queue.push({kind:"emit",method:"item/completed",params:{turnId:"turn-"+n,item:{type:"agentMessage",id:"answer-"+n,text}}});
   control.queue.push({kind:"emit",method:"turn/completed",params:{turn:{id:"turn-"+n,status:"completed"}}});
   await until(async()=>!(await read()).increment.busy,"idle native turn");
  };
  const click=async(action)=>{await page.waitFor('!!document.querySelector(\'[data-increment-action="'+action+'"]:not(:disabled)\')',timeout,action);await page.evaluate('document.querySelector(\'[data-increment-action="'+action+'"]\').click()');};
  const propose=async(format,n)=>{
   await cliJSON(["increment","propose",inc.id],{intent:"Preparare la revisione commerciale con un file "+format+".",stories:[story(format)]});
   await finish(n,"Ho preparato l’intento e una storia. Leggili prima di approvare.");
   await page.waitFor('document.querySelector(".conv-increment")?.textContent.includes("'+format+'")',timeout,"persisted intent");
  };
  const develop=async(format,n)=>{
   await cliJSON(["spec","plan","US-001"],{plan_body:"## Baseline\nhello.txt contains the previous result.\n## Solution\nChange the output and verify it.",tasks:[{id:"TASK-01",title:"Implement",type:"Impl",status:"TODO",dependencies:[],body:taskBody},{id:"TASK-02",title:"Verify",type:"Test",status:"TODO",dependencies:["TASK-01"],body:taskBody}]});
   await cliJSON(["spec","start","US-001"]);
   await fs.writeFile(path.join(inc.workdir,"hello.txt"),"Clienti in "+format+"\n");
   await cliJSON(["task","done","US-001","TASK-01"]);await cliJSON(["task","done","US-001","TASK-02"]);
   await cliJSON(["spec","review","US-001"]);
   await cliJSON(["spec","move","US-001","--to","done"]);
   await cliJSON(["increment","ready",inc.id],{summary:"Apri hello.txt: il risultato concordato è Clienti in "+format+".",checks:["Contenuto del file verificato: Clienti in "+format]});
   await finish(n,"Il risultato è pronto. Provalo e dimmi cosa cambiare.");
   await page.waitFor('!!document.querySelector(\'[data-increment-action="publish"]:not(:disabled)\')',timeout,"business review");
  };
  await propose("CSV",1);
  const parentBefore=await cliJSON(["spec","list"],undefined,sandbox);assert.equal(parentBefore.items.length,0);
  await click("approve-intent");
  await until(()=>control.received.filter(e=>e.kind==="turn/start").length>=2,"development turn");
  await develop("CSV",2);
  assert.equal(await fs.readFile(path.join(sandbox,"hello.txt"),"utf8"),"originale\n");
  assert.equal(await git(["rev-parse","HEAD"]),baseline);
  ok("Intento approvato e sviluppo restano nel worktree e nel backlog interno.");
  const reviewedSHA=(await read()).increment.ready_sha;
  await page.evaluate('(()=>{const input=document.querySelector(".conv-composer-input");input.value="Come posso verificare il risultato?";input.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".conv-composer").requestSubmit();})()');
  await until(()=>control.received.filter(e=>e.kind==="turn/start").length>=3,"question about the result");
  await finish(3,"Apri il file descritto nelle istruzioni di prova.");
  assert.equal((await read()).increment.status,"REVIEW");
  assert.equal((await read()).increment.ready_sha,reviewedSHA);
  ok("Una domanda sul risultato non diventa una richiesta di modifica.");
  await page.evaluate('(()=>{const input=document.querySelector(".conv-composer-input");input.value="Preferisco Excel al CSV";input.dispatchEvent(new Event("input",{bubbles:true}));document.querySelector(".conv-composer").requestSubmit();})()');
  await until(()=>control.received.filter(e=>e.kind==="turn/start").length>=4,"feedback interview");
  await cliJSON(["increment","feedback",inc.id],{feedback:"Preferisco Excel al CSV"});
  await propose("Excel",4);await click("approve-intent");
  await until(()=>control.received.filter(e=>e.kind==="turn/start").length>=5,"second development");
  await develop("Excel",5);
  current=await read();assert.equal(current.session.session.native.id,native);assert.equal(current.increment.id,inc.id);assert.equal(current.increment.revisions.length,2);
  const localStories=await cliJSON(["spec","list"]);assert.equal(localStories.items.length,1);assert.equal(localStories.items[0].code,"US-001");
  ok("Il feedback aggiorna la stessa storia e conserva sessione, sandbox e revisioni.");
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
  await control.close();
  await fs.writeFile(path.join(runDir,"summary.json"),JSON.stringify({ok:!failure,checks,error:failure?.stack},null,2));
  await fs.writeFile(path.join(runDir,"report.html"),"<meta charset=utf-8><title>Increment smoke</title><h1>"+(failure?"FAIL":"PASS")+"</h1><pre>"+checks.map(s=>s.replaceAll("&","&amp;").replaceAll("<","&lt;")).join("\n")+"</pre>");
 }
 console.log("Report: "+runDir);
 if(failure)throw failure;
}
main().catch(error=>{console.error(error.stack);process.exitCode=1;});
