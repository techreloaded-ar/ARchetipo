import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const preview=require('../../cli/internal/web/assets/preview.js');

test('environment approval renders only provider options and escapes commands',()=>{
 const html=preview.renderAttention({kind:'approval',title:'Serve consenso',conversation_id:'env',approvals:[{id:'17',title:'Aprire <Docker>?',args:{command:'<script>bad</script>'},options:[{id:'provider-choice',label:'Solo adesso',description:'Nessuna regola persistente'}]}]});
 assert.match(html,/data-environment-approval="17"/);
 assert.match(html,/data-option="provider-choice"/);
 assert.match(html,/Solo adesso/);
 assert.doesNotMatch(html,/<script>/);
 assert.doesNotMatch(html,/Consenti sempre/);
});

test('environment questions preserve native identifiers and secret field type',()=>{
 const html=preview.renderAttention({kind:'input',title:'Rispondi',inputs:[{id:'input-9',questions:[{id:'choice',header:'Scelta',prompt:'Quale?',multiple:true,allow_text:true,secret:true,options:[{value:'one',label:'Uno'}]}]}]});
 assert.match(html,/data-environment-input="input-9"/);
 assert.match(html,/data-question="choice"/);
 assert.match(html,/type="checkbox"/);
 assert.match(html,/type="password"/);
 assert.equal(preview.renderAttention(null),'');
});

test('environment check labels distinguish historical results without inventing success',()=>{
 const html=preview.renderChecks({checks_label:'Da riconfermare',checks:[{name:'tests',label:'Ultimo esito fallito',report:{id:'r'}}],environment:{previous_error:'Tentativo <precedente>',conversation_id:'env'}});
 assert.match(html,/Ultimo esito fallito/);
 assert.match(html,/data-environment-log="tests"/);
 assert.match(html,/Tentativo &lt;precedente&gt;/);
 assert.match(html,/Apri chat ambiente/);
});

test('cumulative comments carry page and element context as ordinary text',()=>{
 const element={tag:'button',text:'<script>bad</script>',label:'Save',testid:'save',role:'button',page:'Settings',url:'http://127.0.0.1:3000/settings',path:'form > button[save]'};
 const message=preview.commentMessage([{element,note:'  Rinomina in Salva  '},{element:{...element,text:'Cancel'},note:'Sposta a sinistra'}]);
 assert.match(message,/1\. Pagina: Settings/);assert.match(message,/2\. Pagina: Settings/);
 assert.match(message,/data-testid: "save"/);assert.match(message,/Richiesta: Rinomina in Salva/);
 assert.match(message,/form > button\[save\]/);assert.match(message,/http:\/\/127.0.0.1:3000\/settings/);
});
test('picker payload has a bounded explicit shape',()=>{
 assert.equal(preview.cleanElement(null),null);
 assert.equal(preview.cleanElement({tag:'button'}),null);
 const element=preview.cleanElement({tag:'button',url:'/page',text:'x'.repeat(2000),selector:'x'.repeat(3000),evil:'ignored'});
 assert.equal(element.selector.length,2000);
 assert.equal(preview.cleanElement({tag:"button",url:"/old"}).selector,"");
 assert.equal(element.text.length,1000);assert.equal(element.evil,undefined);
});
test('the browser does not own agent setup instructions',()=>{
 assert.equal(preview.setupMessage,undefined);
});

test('panel limits retain the live page at small and large available heights',()=>{
 assert.deepEqual(preview.panelLimits(500.9),{min:160,max:500});
 assert.deepEqual(preview.panelLimits(90),{min:90,max:90});
 assert.deepEqual(preview.panelLimits(-30),{min:0,max:0});
 assert.equal(preview.panelHeight(20,preview.panelLimits(500)),160);
 assert.equal(preview.panelHeight(800,preview.panelLimits(500)),500);
 assert.equal(preview.panelHeight(270,preview.panelLimits(90)),90);
});
test('panel transitions restore the chosen height without mutating another conversation',()=>{
 const first=preview.panelState({}, {height:320});
 const second=preview.panelState({}, {});
 const collapsed=preview.panelState(first,{mode:'collapsed'});
 assert.equal(collapsed.height,320);
 const output=preview.panelState(collapsed,{tab:'output'});
 assert.deepEqual(output,{tab:'output',mode:'expanded',height:320});
 const hidden=preview.panelState(output,{mode:'hidden'});
 assert.deepEqual(preview.panelState(hidden,{mode:'expanded'}),output);
 assert.deepEqual(second,{tab:'comments',mode:'expanded',height:270});
 assert.deepEqual(first,{tab:'comments',mode:'expanded',height:320});
});
