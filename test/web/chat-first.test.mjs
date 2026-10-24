import {createRequire} from 'node:module';
import {test} from 'node:test';
import assert from 'node:assert/strict';
const require=createRequire(import.meta.url);
const {renderConversation}=require('../../cli/internal/web/assets/conversation.js');

test('work entry renders a writable request directly from the server offer',()=>{
 const ui={activities:{start_work:{runnable:true,placeholder:'Describe <your app>'},items:[]}};
 const html=renderConversation({available:true},'My idea',ui);
 assert.match(html,/Quale problema vuoi risolvere/);
 assert.match(html,/conv-composer-input/);
 assert.match(html,/Describe &lt;your app&gt;/);
 assert.match(html,/>My idea<\/textarea>/);
 assert.doesNotMatch(html,/<textarea[^>]*disabled/);
});

test('unavailable work entry preserves the draft and explains the refusal',()=>{
 const html=renderConversation({available:true},'Keep this',{activities:{start_work:{runnable:false,unavailable_reason:'Agent <offline>'},items:[]}});
 assert.match(html,/Agent &lt;offline&gt;/);
 assert.match(html,/<textarea[^>]*disabled>Keep this/);
});

test('product details show specification and learning without synthetic stories or approval',()=>{
 const html=renderConversation({available:true,conversation:{id:'c',state:'ACTIVE'},increment:{id:'work',development_mode:'continuous',title:'Booking',status_label:'Da provare',stories:[],revisions:[{number:1,intent:'Problema e comportamento',stories:[],approved:false}],learning:[{observation:'Osservazione <reale>',decision:'Semplificare'}],actions:[]}},'',{incrementSections:{'panel-work':true}});
 assert.match(html,/Specifica/);
 assert.match(html,/Osservazioni e decisioni/);
 assert.match(html,/Osservazione &lt;reale&gt;/);
 assert.doesNotMatch(html,/Storie dell’incremento|da approvare|Le storie appariranno/);
});
