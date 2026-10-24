import {createRequire} from 'node:module';
import {test} from 'node:test';
import assert from 'node:assert/strict';
const require=createRequire(import.meta.url);
const {renderConversation}=require('../../cli/internal/web/assets/conversation.js');

test('work entry renders a writable request directly from the server offer',()=>{
 const ui={activities:{start_work:{runnable:true,placeholder:'Describe <your app>'},items:[]}};
 const html=renderConversation({available:true},'My idea',ui);
 assert.match(html,/Cosa vuoi costruire/);
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
