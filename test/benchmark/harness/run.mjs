// Real CLI/process/HTTP benchmark. No AI credentials, mocked CLI or fixed sleeps
// used as a success oracle. This measures deterministic overhead, not LLM speed.
import { execFileSync, spawn } from 'node:child_process';
import { mkdirSync, readFileSync, writeFileSync, copyFileSync, existsSync, unlinkSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import assert from 'node:assert/strict';

const repo = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const output = join(repo, 'test/workspaces/harness-benchmark/runs', new Date().toISOString().replaceAll(/[:.]/g, '-'));
const project = join(output, 'project');
const bin = join(output, process.platform === 'win32' ? 'archetipo.exe' : 'archetipo');
mkdirSync(join(project, '.archetipo'), { recursive: true });
mkdirSync(join(project, 'src'));
execFileSync('go', ['build', '-o', bin, './cmd/archetipo'], { cwd: join(repo, 'cli'), stdio: 'inherit' });
writeFileSync(join(project, '.archetipo/config.yaml'), 'connector: file\npaths:\n  test_results: docs/test-results/\nfile:\n  backlog: .archetipo/backlog.yaml\n');
writeFileSync(join(project, 'src/message.txt'), 'hello bicycle');
writeFileSync(join(project, 'deps.lock'), 'one');
copyFileSync(join(repo, 'test/benchmark/harness/fixture.mjs'), join(project, 'fixture.mjs'));
const command = (name, action, timeout_seconds = 5) => ({ name, argv: [process.execPath, 'fixture.mjs', action], timeout_seconds });
const recipe = {
  version: 1, inputs: ['deps.lock', 'fixture.mjs'], outputs: ['.archetipo/environment/dependencies-ready'],
  prepare: [command('start', 'prepare')], probe: [command('ready', 'probe')], cleanup: [command('stop', 'stop')],
  checks: {
    functional: { ...command('http-behavior', 'check'), inputs: ['src', 'fixture.mjs'], cache: true },
    slow: { ...command('slow', 'slow', 1), inputs: ['src', 'fixture.mjs'], cache: false },
  },
};
writeFileSync(join(project, '.archetipo/environment.json'), JSON.stringify(recipe, null, 2));
const rows = [];
function invoke(label, args, expectedError) {
  const start = performance.now();
  let stdout, stderr, exit = 0;
  try { stdout = execFileSync(bin, ['-C', project, 'execution', 'environment', ...args], { encoding: 'utf8', timeout: 15000 }); }
  catch (error) { stdout = String(error.stdout || ''); stderr = String(error.stderr || ''); exit = error.status; }
  const elapsed_ms = Math.round(performance.now() - start);
  const envelope = JSON.parse(expectedError ? stderr : stdout);
  if (expectedError) assert.equal(envelope.error?.code, expectedError);
  else { assert.equal(exit, 0, stderr); assert.equal(envelope.kind, 'verification_result'); }
  const row = { label, elapsed_ms, exit, envelope };
  rows.push(row);
  console.log(`${label}: ${elapsed_ms} ms; passed=${envelope.data?.passed}; reused=${envelope.data?.reused}`);
  return envelope.data || envelope.error;
}
const check = (label, ...extra) => invoke(label, ['check', 'functional', '--spec', 'US-001', ...extra]);
const counts = () => JSON.parse(readFileSync(join(project, '.archetipo/environment/counts.json')));
const fixtureEnv = () => ({ ...process.env, ARCHETIPO_ENVIRONMENT_STATE: join(project, '.archetipo/environment'), ARCHETIPO_ENVIRONMENT_ID: JSON.parse(readFileSync(rows.find(r => r.envelope.data?.environment_id).envelope.data.report_path)).environment_id });
let failure;
try {
  let r = check('cold'); assert(r.passed && r.stable && !r.reused); assert.equal(counts().prepare, 1);
  for (let n = 1; n <= 5; n++) {
    r = check(`warm-${n}`); assert(r.passed && r.stable && r.reused); assert.equal(counts().prepare, 1); assert.equal(counts().check, 1);
    assert(rows.at(-1).elapsed_ms < 2000, 'warm path exceeded 2 seconds');
  }
  writeFileSync(join(project, 'src/message.txt'), 'new bicycle candidate');
  r = check('changed-source'); assert(r.passed && !r.reused); assert.equal(counts().prepare, 1); assert.equal(counts().check, 2);
  writeFileSync(join(project, 'deps.lock'), 'two');
  r = check('changed-dependencies'); assert(r.passed && !r.reused); assert.equal(counts().prepare, 2);
  execFileSync(process.execPath, ['fixture.mjs', 'crash'], { cwd: project, env: fixtureEnv(), stdio: 'pipe' });
  r = check('dead-service-recovery'); assert(r.passed && !r.reused); assert.equal(counts().prepare, 3);
  writeFileSync(join(project, 'src/fail.txt'), 'force a real failed assertion');
  r = check('failed-check'); assert(!r.passed && !r.reused); assert.equal(r.steps[0].exit_code, 7);
  const failedLog = r.steps[0].log_path;
  r = check('failed-check-repeated'); assert(!r.passed && !r.reused); assert(existsSync(failedLog));
  unlinkSync(join(project, 'src/fail.txt'));
  r = check('fixed-check'); assert(r.passed && !r.reused);

  const child = spawn(bin, ['-C', project, 'execution', 'environment', 'check', 'slow', '--spec', 'US-001'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let out = '', err = '';
  child.stdout.on('data', data => { out += data; }); child.stderr.on('data', data => { err += data; });
  const ended = new Promise((resolve, reject) => { child.on('error', reject); child.on('exit', code => code === 0 ? resolve() : reject(new Error(err))); });
  const deadline = performance.now() + 5000;
  while (!existsSync(join(project, '.archetipo/environment/descendant.pid'))) {
    assert(performance.now() < deadline, 'slow command did not start');
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  invoke('concurrent-operation', ['prepare'], 'E_CONFLICT');
  await ended;
  r = JSON.parse(out).data; assert(!r.passed && r.steps[0].exit_code === -1);
  rows.push({ label: 'timeout-descendants', elapsed_ms: r.duration_ms, envelope: { data: r } });
  assert(r.duration_ms < 4000, 'timeout did not bound the process tree');
  // A descendant that survived cancellation would keep updating this heartbeat.
  const beat = join(project, '.archetipo/environment/heartbeat');
  const before = readFileSync(beat, 'utf8');
  await new Promise(resolve => setTimeout(resolve, 120));
  assert.equal(readFileSync(beat, 'utf8'), before, 'cancelled command leaked a live descendant');
  unlinkSync(join(project, '.archetipo/environment/descendant.pid'));
  const crashStart = performance.now();
  const crash = spawn(bin, ['-C', project, 'execution', 'environment', 'check', 'slow', '--spec', 'US-001'], { stdio: 'ignore' });
  const crashed = new Promise(resolve => crash.on('exit', resolve));
  while (!existsSync(join(project, '.archetipo/environment/descendant.pid'))) {
    assert(performance.now() - crashStart < 5000, 'hard-kill fixture did not start');
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  crash.kill('SIGKILL');
  await crashed;
  // Wait for the owned supervisor to release its command lock, not a fixed
  // delay that could hide an orphan. Recovery is required within four seconds.
  const commandLock = join(project, '.archetipo/conversations/verification-command.lock');
  while (existsSync(commandLock)) {
    assert(performance.now() - crashStart < 4000, 'supervisor did not end after parent death');
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  const hardBeat = readFileSync(beat, 'utf8');
  await new Promise(resolve => setTimeout(resolve, 120));
  assert.equal(readFileSync(beat, 'utf8'), hardBeat, 'CLI hard kill leaked a live descendant');
  r = check('hard-kill-recovery'); assert(r.passed && r.reused);
  rows.push({ label: 'hard-kill-no-orphan', elapsed_ms: Math.round(performance.now() - crashStart), envelope: { data: { passed: true, reused: false } } });
  invoke('cleanup', ['cleanup']);
  r = check('after-cleanup'); assert(r.passed && !r.reused); assert.equal(counts().prepare, 4);
  // A real parent dies while a compatible check uses its service. Its private
  // pipe must cancel the command tree before the guard runs registered cleanup.
  recipe.checks.functional.during_preview = true;
  recipe.checks.slow.during_preview = true;
  recipe.checks.slow.timeout_seconds = 30;
  writeFileSync(join(project, '.archetipo/environment.json'), JSON.stringify(recipe, null, 2));
  const liveOwner = spawn(process.execPath, ['-e', `
    const {spawn}=require('node:child_process');
    const guard=spawn(process.argv[1],['execution','environment','_guard'],{stdio:['pipe','pipe','inherit']});
    guard.stdin.write(JSON.stringify({root:process.argv[2]})+'\\n');
    guard.stdout.pipe(process.stdout); guard.on('exit',code=>process.exit(code||0));
  `, bin, project], {cwd:project, stdio:['ignore','pipe','pipe']});
  const ownerExit = new Promise(resolve => liveOwner.once('exit',resolve));
  let ownerReady=''; liveOwner.stdout.on('data',data=>{ownerReady+=data;});liveOwner.stderr.resume();
  const waitFor = async (predicate,label,start=performance.now()) => {
    while(!predicate()) {
      assert(performance.now()-start<10000,label);
      await new Promise(resolve=>setTimeout(resolve,10));
    }
  };
  let live;
  try {
    await waitFor(()=>ownerReady.includes('\n'),'live owner did not start');
    assert.deepEqual(JSON.parse(ownerReady.trim()),{});
    const installed=counts().prepare;
    r=check('live-preview-check');assert(r.passed && r.stable);assert.equal(counts().prepare,installed);
    const descendantFile=join(project,'.archetipo/environment/descendant.pid');
    if(existsSync(descendantFile))unlinkSync(descendantFile);
    live=spawn(bin,['-C',project,'execution','environment','check','slow','--fresh'],{stdio:['ignore','pipe','pipe']});
    let liveOutput='',ended=false;
    live.stdout.on('data',data=>{liveOutput+=data;});live.stderr.resume();live.once('exit',()=>{ended=true;});
    const liveStart=performance.now();
    await waitFor(()=>existsSync(descendantFile),'live check did not start',liveStart);
    const pid=Number(readFileSync(descendantFile,'utf8'));
    liveOwner.kill('SIGKILL');await ownerExit;
    await waitFor(()=>ended,'live check survived owner death',liveStart);
    const outcome=JSON.parse(liveOutput).data;assert.equal(outcome.passed,false);
    await waitFor(()=>!existsSync(join(project,'.archetipo/environment/owned-server.json')),'guard cleanup did not finish',liveStart);
    assert.throws(()=>process.kill(pid,0),'live descendant survived owner death');
    assert.equal(JSON.parse(readFileSync(join(project,'.archetipo/environment/state.json'),'utf8')).preparation_id,'');
    rows.push({label:'live-check-owner-hard-death',elapsed_ms:Math.round(performance.now()-liveStart),exit:0,envelope:{data:outcome}});
  } finally {liveOwner.kill('SIGTERM');live?.kill('SIGTERM');}

} catch (error) { failure = error; }
finally {
  try { invoke('final-cleanup', ['cleanup']); } catch (error) { failure ||= error; }
  const report = { success: !failure, failure: failure?.stack, project, rows, counts: existsSync(join(project, '.archetipo/environment/counts.json')) ? counts() : {} };
  writeFileSync(join(output, 'summary.json'), JSON.stringify(report, null, 2));
  const escape = value => String(value).replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('"', '&quot;');
  writeFileSync(join(output, 'report.html'), `<!doctype html><meta charset="utf-8"><title>ARchetipo harness benchmark</title><style>body{font:16px system-ui;max-width:1100px;margin:3rem auto;padding:1rem}td,th{padding:.6rem;text-align:left;border-bottom:1px solid #ccc}pre{white-space:pre-wrap}</style><h1>Harness benchmark: ${report.success ? 'PASS' : 'FAIL'}</h1><p>Real CLI, local HTTP service and child processes. No AI: these timings measure deterministic overhead only.</p><table><tr><th>Case</th><th>ms</th><th>Passed</th><th>Reused</th></tr>${rows.map(row => `<tr><td>${escape(row.label)}</td><td>${row.elapsed_ms}</td><td>${row.envelope.data?.passed ?? row.envelope.error?.code}</td><td>${row.envelope.data?.reused ?? ''}</td></tr>`).join('')}</table><h2>Evidence</h2>${rows.filter(row => row.envelope.data?.report_path).map(row => `<p><a href="${escape(row.envelope.data.report_path)}">${escape(row.label)}</a></p>`).join('')}<pre>${escape(report.failure || '')}</pre>`);
  console.log(`Report: ${join(output, 'report.html')}`);
}
if (failure) throw failure;
