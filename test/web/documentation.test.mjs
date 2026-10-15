import { test } from 'node:test';
import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
const require = createRequire(import.meta.url);
const { pageURL, wikiLink, renderIndex, renderPage } = require('../../cli/internal/web/assets/documentation.js');

test('Wiki links resolve relative to the page or Wiki root, preserving fragments', () => {
 assert.deepEqual(wikiLink('engineering/code-map.md', '../overview.md#scopo'), {id:'overview', anchor:'scopo'});
 assert.deepEqual(wikiLink('engineering/code-map.md', '/architecture/context-map.md'), {id:'architecture/context-map', anchor:''});
 assert.deepEqual(wikiLink('overview.md', 'guide%20utente.md'), {id:'guide utente', anchor:''});
 for (const href of ['../../private.md', 'https://example.org/doc.md', '//example.org/doc.md', 'javascript:evil.md', 'file:///private.md', '%ZZ.md', '..\\private.md']) assert.equal(wikiLink('overview.md', href), null, href);
 assert.equal(pageURL('wiki', 'engineering/code-map', 'scopo'), '?view=documentation&collection=wiki&page=engineering%2Fcode-map#scopo');
});

test('index groups pages and echoes server review labels and collection identity', () => {
 const html = renderIndex([{id:'wiki',label:'Wiki',available:true,items:[{id:'engineering/code-map',meta:{title:'<img src=x onerror=evil()>'},state_label:'Etichetta del server'}]}], 'wiki', 'engineering/code-map');
 assert.match(html, /aria-current="page"/);
 assert.match(html, /Etichetta del server/);
 assert.match(html, /engineering/);
 assert.doesNotMatch(html, /<img/);
 const future = renderIndex([{id:'manuals', label:'Manuali', available:true, items:[{id:'guide', meta:{title:'Guida'},state_label:''}]}], '', '');
 assert.match(future, /collection=manuals/);
 assert.match(future, /Manuali/);
});

test('empty Wiki and no search matches have distinct messages', () => {
 assert.match(renderIndex([{id:'wiki',label:'Wiki',available:false,items:[]}]), /non è ancora stata generata/);
 assert.match(renderIndex([{id:'wiki',label:'Wiki',available:true,items:[]}]), /Nessuna pagina trovata/);
});

test('page presents provenance and issues without interpreting review states', () => {
 const html = renderPage({id:'overview',path:'overview.md',state_label:'Stato ricevuto',meta:{title:'Titolo',type:'overview',sources:[{path:'<script>',role:'implementation'}],issues:[{code:'GAP',summary:'Evidenza mancante'}],review:{reviewed_at:'2026-10-15'}},body:'body'}, value => `<p>${value}</p>`);
 assert.match(html, /Stato ricevuto/);
 assert.match(html, /Evidenza mancante/);
 assert.match(html, /2026-10-15/);
 assert.match(html, /&lt;script&gt;/);
 assert.match(html, /<p>body<\/p>/);
});

test('Wiki controls echo server actions, availability and placement without skill terminology', () => {
 const { renderActions, renderActivities } = require('../../cli/internal/web/assets/documentation.js');
 const actions = [{id:'future-create',label:'Crea conoscenza',placement:'primary',runnable:true},{id:'future-check',label:'Verifica contenuti',placement:'menu',runnable:false,unavailable_reason:'Motivo <sicuro>'},{id:'future-query',label:'Domanda sulla pagina',placement:'page',runnable:true}];
 const toolbar = renderActions(actions);
 assert.match(toolbar, /data-doc-action="future-create"/);
 assert.match(toolbar, /Altre azioni/);
 assert.match(toolbar, /data-doc-action="future-check" disabled/);
 assert.match(toolbar, /Motivo &lt;sicuro&gt;/);
 assert.doesNotMatch(toolbar, /future-query|archetipo-wiki/);
 const page = renderPage({id:'engineering/code-map',meta:{}},()=>'',actions);
 assert.match(page, /data-doc-action="future-query" data-doc-action-page="engineering\/code-map"/);
 assert.doesNotMatch(page, /future-create|future-check/);
 const activities = renderActivities([{label:'Aggiornamento Wiki',status_label:'In attesa della risposta',conversation_id:'conv-123',changed_pages:['overview','bad"<id>']}]);
 assert.match(activities, /data-doc-conversation="conv-123"/);
 assert.match(activities, /Pagine cambiate \(2\)/);
 assert.match(activities, /bad&quot;&lt;id&gt;/);
 assert.doesNotMatch(activities, /<id>/);
});
