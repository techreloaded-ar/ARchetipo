import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {renderConversation}=require('../../cli/internal/web/assets/conversation.js');

test('increment deletion is offered only with the label supplied by the server',()=>{
 const base={available:true,conversation:{id:'conv-test',state:'ACTIVE'},events:[],approvals:[],runs:[],increment:{id:'inc-test',title:'A result',stories:[],revisions:[],actions:[]}};
 const ui={incrementSections:{'panel-inc-test':true}};
 assert.doesNotMatch(renderConversation(base,'',ui),/data-increment-delete/);
 const offered={...base,increment:{...base.increment,delete_label:'Remove <local> environment'}};
 const rendered=renderConversation(offered,'',ui);
 assert.match(rendered,/data-increment-delete/);assert.match(rendered,/Remove &lt;local&gt; environment/);
 assert.match(renderConversation(offered,'',{...ui,busy:true}),/data-increment-delete disabled/);
 assert.doesNotMatch(renderConversation(base,'',ui),/Elimina incremento/);
});

// Il passo di rimozione lo decide il server, per esempio quando la PR è stata
// chiusa senza merge: il renderer lo disegna tra i prossimi passi con le parole
// ricevute e lo lega alla stessa rimozione del pannello.
test('the removal step offered by the server is drawn among the next steps',()=>{
 const increment={id:'inc-closed',title:'A result',stories:[],revisions:[],actions:[],next_steps_note:'received <note>',next_steps:[{kind:'action',action:'refresh',label:'Refresh received'},{kind:'delete',tone:'warn',label:'Remove <received>'}]};
 const view={available:true,conversation:{id:'conv-test',state:'ACTIVE'},events:[],approvals:[],runs:[],increment};
 const row=html=>html.slice(html.indexOf('<section class="conv-increment conv-next"'),html.indexOf('</section>',html.indexOf('<section class="conv-increment conv-next"')));
 const rendered=row(renderConversation(view,'',{}));
 assert.match(rendered,/class="conv-step conv-step-action is-warn" data-increment-delete>Remove &lt;received&gt;</);
 assert.match(rendered,/received &lt;note&gt;/);
 assert.doesNotMatch(rendered,/data-increment-step-action="(?!refresh)/);
 // The agent working does not take the way out away; a command in flight does.
 assert.match(row(renderConversation({...view,increment:{...increment,busy:true}},'',{})),/data-increment-delete>/);
 assert.match(row(renderConversation(view,'',{busy:true})),/data-increment-delete disabled>/);
 assert.doesNotMatch(row(renderConversation({...view,increment:{...increment,next_steps:increment.next_steps.slice(0,1)}},'',{})),/data-increment-delete/);
});
