#!/usr/bin/env node
// Real browser, viewer, native Codex adapter, journal and Wiki service. Only
// the agent is fake; every emitted answer is controlled by this smoke.
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import http from 'node:http';
import path from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';
import { apiJSON, buildCLI, createRunDir, makeRunCommand, readBody, startViewServer, stopProcess } from './support/view-smoke-harness.mjs';
import { findChrome, launchChrome } from './viewer-page-load-smoke.mjs';

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const cli = path.join(repoRoot, 'test/e2e/.bin/archetipo');
const checks = [];
const ok = text => { checks.push(text); console.log(`-> ok: ${text}`); };
const post = body => ({method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
async function until(check, description) {
 const started = Date.now();
 while (Date.now() - started < 15000) { const result = await check(); if (result) return result; await delay(50); }
 throw new Error(`Timeout: ${description}`);
}
async function controlServer() {
 const commands=[],received=[];
 let activePID;
 const server=http.createServer(async(req,res)=>{
  let body={};
  if(req.url.startsWith('/next')) { const pid=Number(new URL(req.url,'http://localhost').searchParams.get('pid')); const i=commands.findIndex(c=>c.pid===pid); body=i<0?{kind:'none'}:commands.splice(i,1)[0]; }
  else if(req.url==='/received') {received.push(JSON.parse(await readBody(req)));body={ok:true};}
  res.writeHead(200,{'Content-Type':'application/json'});res.end(JSON.stringify(body));
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 return {url:`http://127.0.0.1:${server.address().port}`,received,
  emit(method,params){commands.push({kind:'emit',method,params,pid:activePID});},
  async turn(count){const turn=await until(()=>received.filter(r=>r.kind==='turn/start')[count-1],`turno ${count} ricevuto dall’agente`);activePID=turn.pid;return turn;},
  async close(){server.closeAllConnections();await new Promise(resolve=>server.close(resolve));}};
}

async function main(){
 if(process.platform==='win32'){console.log('SKIP: il binario simulato richiede una shebang POSIX');return;}
 const chrome=await findChrome();if(!chrome)throw new Error('Chrome richiesto per provare i clic Wiki: imposta ARCHETIPO_CHROME.');
 const dir=await createRunDir(path.join(repoRoot,'test/workspaces/wiki-from-view-smoke'));
 const sandbox=path.join(dir,'sandbox');await fs.mkdir(sandbox,{recursive:true});
 const control=await controlServer();
 const env={...process.env,ARCHETIPO_DATA_DIR:repoRoot,ARCHETIPO_STATE_DIR:path.join(dir,'state'),FAKE_CODEX_CONTROL:control.url};
 const run=makeRunCommand(env);
 let view,browser,page,failure;
 try {
  await buildCLI(cli,repoRoot,run);
  await run('init',cli,['init','--tool','codex','--connector','file','--yes'],{cwd:sandbox});
  await fs.mkdir(path.join(sandbox,'src'),{recursive:true});
  await fs.writeFile(path.join(sandbox,'src/index.js'),'export const greeting = "Hello";\n');
  await fs.writeFile(path.join(sandbox,'package.json'),JSON.stringify({name:'wiki-smoke',type:'module'}));
  await run('git-init','git',['init','-b','main'],{cwd:sandbox});
  await run('git-add','git',['add','src','package.json'],{cwd:sandbox});
  await run('git-baseline','git',['-c','user.name=Wiki Smoke','-c','user.email=wiki-smoke@example.test','commit','-m','test: seed Wiki evidence'],{cwd:sandbox});
  view=await startViewServer(cli,sandbox,env,'/api/documentation');
  await apiJSON(`${view.url}/api/execution/provider/default`,{method:'PUT',headers:{'Content-Type':'application/json'},body:JSON.stringify({id:'codex',config:{command:path.join(repoRoot,'test/e2e/support/fake-codex.mjs'),model:'gpt-fake',sandbox:'workspace-write'}})});
  browser=await launchChrome(chrome,path.join(dir,'chrome'));
  page=await browser.newPage();const exceptions=[];
  page.on('Runtime.exceptionThrown',event=>exceptions.push(event.exceptionDetails));
  await page.send('Runtime.enable');await page.send('Page.enable');
  await page.send('Page.navigate',{url:view.url});await page.once('Page.loadEventFired',30000,'pagina Wiki');
  await page.waitFor(`document.querySelector('[data-shell-view="documentation"]')`,10000,'vista Documentazione');
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click()`);
  await page.waitFor(`document.querySelector('[data-doc-action="wiki-create"]:enabled')`,10000,'Crea Wiki disponibile');
  assert.equal(await page.evaluate(`document.getElementById('workspace-documentation').textContent.includes('archetipo-wiki')`),false);
  await page.evaluate(`document.querySelector('[data-doc-action="wiki-create"]').click()`);
  const create=await control.turn(1);
  assert.match(JSON.stringify(create.params.input),/archetipo-wiki/);
  assert.match(JSON.stringify(create.params.input),/bootstrap/);
  await page.waitFor(`document.getElementById('workspace-shell').dataset.view === 'conversation'`,10000,'conversazione dedicata alla creazione');
  ok('Crea Wiki apre una sessione nativa dedicata e consegna bootstrap senza esporre la skill nell’interfaccia');
  // The smoke writes the artifacts, like an agent would, using the exact page
  // format consumed by the CLI. It never simulates the viewer or Wiki reads.
  await run('wiki-init',cli,['wiki','init'],{cwd:sandbox});
  await fs.mkdir(path.join(sandbox,'src'),{recursive:true});
  await fs.writeFile(path.join(sandbox,'src/index.js'),'export const greeting = "Hello";\n');
  const root=path.join(sandbox,'docs/wiki');
  for(const [id,type] of [['overview','overview'],['architecture/context-map','context-map'],['engineering/code-map','code-map'],['operations/development','operations']]){
   const coverage=type==='code-map'?'coverage:\n  - kind: boundary\n    path: .\n    status: mapped\n    pages: [overview]\n  - kind: boundary\n    path: src\n    status: mapped\n    pages: [overview]\n':'';
   const sections=type==='context-map'?['contexts','relationships','shared','uncertainties']:type==='code-map'?['domain-code','shared','unmapped','coverage']:[];
   const markers=sections.map(section=>`<!-- archetipo:wiki section=${section} -->\n## ${section}\n\nContenuto verificabile.\n`).join('\n');
   const link=id==='overview'?'[Mappa](architecture/context-map.md)':'[Panoramica](../overview.md)';
   await fs.mkdir(path.dirname(path.join(root,`${id}.md`)),{recursive:true});
   await fs.writeFile(path.join(root,`${id}.md`),`---\ntype: ${type}\ntitle: ${id}\ndescription: Wiki del progetto\nstatus: generated\nsources:\n  - path: src\n${coverage}---\n# ${id}\n\n${link}\n\n${markers}\nContenuto iniziale.\n`);
  }
  const validation=await run('validate-bootstrap',cli,['wiki','validate','--profile','bootstrap'],{cwd:sandbox});
  assert.equal(JSON.parse(validation.stdout).data.ok,true,validation.stdout);
  async function finish(number,operation){
   const text=JSON.stringify({artifact:'wiki',operation,status:'COMPLETED'});
   control.emit('item/completed',{turnId:`turn-${number}`,item:{id:`answer-${number}`,type:'agentMessage',text}});
   control.emit('turn/completed',{turn:{id:`turn-${number}`,status:'completed'}});
   return until(async()=>{
    const actions=await apiJSON(`${view.url}/api/documentation/wiki/actions`);
    return actions.activities[0]&&!actions.activities[0].running?actions:null;
   },`attività ${operation} conclusa`);
  }
  let result=await finish(1,'bootstrap');
  assert.match(result.activities[0].status_label,/completata/);
  assert.equal(result.activities[0].changed_pages.length,4);
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click()`);
  await page.waitFor(`document.querySelector('[data-doc-action="wiki-refresh"]:enabled') && document.querySelector('#docs-content h1')?.textContent==='overview'`,15000,'Wiki generata nel lettore');
  assert.match(await page.evaluate(`document.getElementById('docs-activities').textContent`),/Pagine cambiate/);
  ok('La ricevuta viene verificata dalla Wiki persistita e il lettore mostra le quattro pagine generate');
  await page.evaluate(`document.querySelector('[data-doc-action="wiki-refresh"]').click()`);
  const refresh=await control.turn(2);
  await page.waitFor(`document.getElementById('workspace-shell').dataset.view==='conversation'`,10000,'conversazione di aggiornamento');
  assert.match(JSON.stringify(refresh.params.input),/refresh/);
  const overview=path.join(root,'overview.md');const before=await fs.readFile(overview,'utf8');
  await fs.writeFile(overview,before.replace('iniziale','aggiornato'));
  result=await finish(1,'refresh'); // Each dedicated native app-server starts its own turn numbering.
  assert.deepEqual(result.activities[0].changed_pages,['overview']);
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click()`);
  await page.waitFor(`document.querySelector('[data-doc-action="wiki-query"][data-doc-action-page="overview"]:enabled')`,15000,'domanda sulla pagina selezionata');
  await page.evaluate(`document.querySelector('[data-doc-action="wiki-query"]').click()`);
  const query=await control.turn(3);
  await page.waitFor(`document.getElementById('workspace-shell').dataset.view==='conversation'`,10000,'conversazione sulla pagina');
  assert.match(JSON.stringify(query.params.input),/exact Wiki ID is overview/);
  await finish(1,'query');
  assert.equal(await fs.readFile(overview,'utf8'),before.replace('iniziale','aggiornato'));
  ok('Aggiorna Wiki riporta solo la pagina cambiata e Chiedi su questa pagina trasferisce il contesto senza modificare i contenuti');
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click()`);
  await page.waitFor(`document.querySelector('[data-doc-action="wiki-lint"]:enabled')`,15000,'Verifica Wiki disponibile');
  await page.evaluate(`document.querySelector('.docs-actions-menu').open=true;document.querySelector('[data-doc-action="wiki-lint"]').click()`);
  const lint=await control.turn(4);
  await page.waitFor(`document.getElementById('workspace-shell').dataset.view==='conversation'`,10000,'conversazione di verifica');
  assert.match(JSON.stringify(lint.params.input),/lint/);
  await finish(1,'lint');
  await page.evaluate(`document.querySelector('[data-shell-view="documentation"]').click()`);
  await page.waitFor(`document.querySelectorAll('.docs-activity').length===4 && document.querySelector('[data-doc-action="wiki-refresh"]:enabled')`,10000,'attività aggiornate e pulsanti nuovamente disponibili');
  const desktop=await page.send('Page.captureScreenshot',{format:'png'});
  await fs.writeFile(path.join(dir,'wiki-documentation-desktop.png'),Buffer.from(desktop.data,'base64'));
  await page.send('Emulation.setDeviceMetricsOverride',{width:390,height:844,deviceScaleFactor:1,mobile:false});
  assert.equal(await page.evaluate(`document.documentElement.scrollWidth > innerWidth`),false);
  const mobile=await page.send('Page.captureScreenshot',{format:'png'});
  await fs.writeFile(path.join(dir,'wiki-documentation-mobile.png'),Buffer.from(mobile.data,'base64'));
  await page.send('Emulation.clearDeviceMetricsOverride');
  await page.send('Page.reload',{ignoreCache:true});await page.once('Page.loadEventFired',30000,'ricaricamento della Wiki');
  await page.waitFor(`document.querySelectorAll('.docs-activity').length===4`,15000,'attività Wiki dal journal dopo reload',()=>page.evaluate(`({view:document.getElementById('workspace-shell').dataset.view,url:location.href,activities:document.querySelectorAll('.docs-activity').length,content:document.getElementById('docs-actions').textContent})`));
  await page.evaluate(`document.querySelector('.docs-activity-history').open=true;document.querySelector('[data-doc-conversation]').click()`);
  await page.waitFor(`document.getElementById('workspace-shell').dataset.view==='conversation'`,10000,'riapertura della conversazione');
  await page.waitFor(`!Array.from(document.querySelectorAll('.conv-event-text')).some(node => node.textContent.includes('\"artifact\":\"wiki\"'))`,10000,'ricevuta tecnica nascosta nella conversazione');
  assert.equal(control.received.filter(r=>r.kind==='turn/start').length,4);
  assert.deepEqual(exceptions,[]);
  await page.send('Page.captureScreenshot',{format:'png'}).then(image=>fs.writeFile(path.join(dir,'wiki-conversation.png'),Buffer.from(image.data,'base64')));
  ok('Verifica Wiki, storico dopo reload e Apri conversazione funzionano senza duplicare turni né chiudere le sessioni');
 }catch(error){failure=error;}finally{
  if(page)await page.close();if(browser)await browser.close();if(view)await stopProcess(view.child,run);await control.close();
  await fs.writeFile(path.join(dir,'summary.json'),JSON.stringify({checks,error:failure?.stack||null},null,2));
  await fs.writeFile(path.join(dir,'report.html'),`<meta charset="utf-8"><title>Wiki smoke</title><pre>${JSON.stringify({checks,error:failure?.stack||null},null,2).replaceAll('&','&amp;').replaceAll('<','&lt;')}</pre>`);
 }
 if(failure)throw failure;
 console.log(`Report: ${dir}`);
}
await main();
