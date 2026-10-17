import { test } from 'node:test';
import assert from 'node:assert/strict';
import { catalogServer } from '../src/catalog.mjs';

async function fixture(t) {
  const server = catalogServer();
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  t.after(() => new Promise(resolve => server.close(resolve)));
  return `http://127.0.0.1:${server.address().port}`;
}

test('lists existing motorcycle trips', async t => {
  const base = await fixture(t);
  const response = await fetch(base + '/trips');
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json(), [{ id: 1, title: 'Alpi in moto', vehicle: 'motorcycle' }]);
});
test('creates a motorcycle by default and retains it', async t => {
  const base = await fixture(t);
  const response = await fetch(base + '/trips', { method: 'POST', body: JSON.stringify({ title: 'Dolomiti' }) });
  assert.equal(response.status, 201);
  assert.equal((await response.json()).vehicle, 'motorcycle');
  assert.equal((await (await fetch(base + '/trips')).json()).length, 2);
});
test('rejects an empty title and unsupported vehicle', async t => {
  const base = await fixture(t);
  for (const body of [{ title: '' }, { title: 'Unknown', vehicle: 'plane' }]) {
    const response = await fetch(base + '/trips', { method: 'POST', body: JSON.stringify(body) });
    assert.equal(response.status, 400);
  }
});
