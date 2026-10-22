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
