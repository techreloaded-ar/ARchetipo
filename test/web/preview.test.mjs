import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const preview=require('../../cli/internal/web/assets/preview.js');

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
test('preparation asks the agent for an explicit recipe without starting it',()=>{
 const message=preview.setupMessage();assert.match(message,/resources/);assert.match(message,/start, probe, stop, stopped/);assert.match(message,/ARCHETIPO_RESOURCE_OWNER/);assert.match(message,/ARCHETIPO_PORT_<NOME>/);assert.match(message,/archetipo execution preview configure/);assert.match(message,/Non avviare il server/);assert.match(message,/\{port\}/);assert.match(message,/prepare_environment/);assert.match(message,/execution preview prepare/);assert.match(message,/versionati/);assert.match(message,/directory di lavoro/);
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
