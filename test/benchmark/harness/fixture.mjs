import { createServer } from 'node:http';
import { spawn } from 'node:child_process';
import { readFileSync, writeFileSync, existsSync, mkdirSync, unlinkSync } from 'node:fs';
import { join } from 'node:path';
import { randomBytes } from 'node:crypto';
import assert from 'node:assert/strict';

const root = process.cwd();
const dir = process.env.ARCHETIPO_ENVIRONMENT_STATE;
const owner = process.env.ARCHETIPO_ENVIRONMENT_ID;
const ownedFile = join(dir, 'owned-server.json');
const countFile = join(dir, 'counts.json');
const action = process.argv[2];
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
function count(key) {
  const counts = existsSync(countFile) ? JSON.parse(readFileSync(countFile)) : {};
  counts[key] = (counts[key] || 0) + 1;
  writeFileSync(countFile, JSON.stringify(counts));
}
function state() {
  const value = JSON.parse(readFileSync(ownedFile));
  assert.equal(value.owner, owner, 'resource belongs to a different environment');
  return value;
}
async function request(path = '/health') {
  const value = state();
  return fetch(value.url + path, { headers: { 'x-owner': owner, 'x-token': value.token }, signal: AbortSignal.timeout(1000) });
}
async function ready() {
  const response = await request();
  assert.equal(response.status, 200);
  assert.equal((await response.json()).owner, owner);
}
if (action === 'serve') {
  const token = randomBytes(16).toString('hex');
  const server = createServer((request, response) => {
    if (request.headers['x-owner'] !== owner || request.headers['x-token'] !== token) {
      response.writeHead(403).end(); return;
    }
    response.setHeader('content-type', 'application/json');
    if (request.url === '/shutdown') {
      response.end('{}'); server.close(() => process.exit(0)); return;
    }
    response.end(JSON.stringify({ owner, message: readFileSync(join(root, 'src/message.txt'), 'utf8') }));
  });
  server.listen(0, '127.0.0.1', () => {
    writeFileSync(ownedFile, JSON.stringify({ owner, token, pid: process.pid, url: `http://127.0.0.1:${server.address().port}` }));
  });
} else if (action === 'prepare') {
  count('prepare');
  if (existsSync(ownedFile)) {
    state(); // An unknown owner never permits cleanup or replacement.
    try { await ready(); process.exit(0); } catch { /* recreate our dead service */ }
  }
  const child = spawn(process.execPath, [process.argv[1], 'serve'], { env: process.env, cwd: root, detached: true, stdio: 'ignore' });
  child.unref();
  let started = false;
  for (let n = 0; n < 100; n++) {
    try { await ready(); started = true; break; } catch { await pause(20); }
  }
  assert(started, 'owned local service did not become ready');
  writeFileSync(join(dir, 'dependencies-ready'), 'local dependencies');
} else if (action === 'probe') {
  count('probe'); await ready();
} else if (action === 'check') {
  count('check');
  const response = await request('/value');
  assert.equal(response.status, 200);
  assert.equal((await response.json()).message, readFileSync(join(root, 'src/message.txt'), 'utf8'));
  if (existsSync(join(root, 'src/fail.txt'))) {
    console.error('real verification failure'); process.exit(7);
  }
  console.log('HTTP behavior verified against this candidate');
} else if (action === 'stop' || action === 'crash') {
  count(action);
  if (existsSync(ownedFile)) {
    await request('/shutdown');
    for (let n = 0; n < 100; n++) {
      try { await ready(); } catch { break; }
      await pause(20);
    }
    if (action === 'stop') unlinkSync(ownedFile);
  }
} else if (action === 'slow') {
  count('slow');
  const child = spawn(process.execPath, [process.argv[1], 'descendant'], { stdio: 'inherit' });
  writeFileSync(join(dir, 'descendant.pid'), String(child.pid));
  console.log(`descendant pid ${child.pid}`);
  await pause(30000);
} else if (action === 'descendant') {
  mkdirSync(dir, { recursive: true });
  setInterval(() => writeFileSync(join(dir, 'heartbeat'), String(Date.now())), 30);
} else {
  throw new Error('unknown fixture action');
}
