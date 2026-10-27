import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const require = createRequire(import.meta.url);
const { renderConversation } = require("../../cli/internal/web/assets/conversation.js");
const base = { available: true, conversation: { id: "conv-1", state: "ACTIVE" }, events: [] };
const revision = { number: 2, intent: "Un risultato concordato", approved: false, stories: [{code:"US-001",title:"Esportare i clienti",body:"Criteri leggibili"}] };

test("l’incremento usa le azioni e lo stato del server, preservando il compositore libero", () => {
 const html = renderConversation({...base, increment:{id:"inc-1",title:"Esportazione",request:"La richiesta",status_label:"Etichetta fornita",note:"Nota fornita",revisions:[revision],actions:[{id:"azione-mai-vista",label:"Comando fornito"}]}}, "la mia bozza", {incrementSections:{"panel-inc-1":true}});
 assert.match(html,/Etichetta fornita/);
 assert.match(html,/data-increment-action="azione-mai-vista"/);
 assert.match(html,/Comando fornito/);
 assert.match(html,/Nota fornita/);
 assert.match(html,/la mia bozza/);
 assert.match(html,/Intento e storie/);
 assert.match(html,/Esportare i clienti/);
});
test("la conversazione ordinaria non acquisisce pannelli o approvazioni di incremento", () => {
 const html = renderConversation(base,"posso sviluppare liberamente",{});
 assert.doesNotMatch(html,/class="conv-increment"/);
 assert.doesNotMatch(html,/data-increment-action/);
 assert.match(html,/posso sviluppare liberamente/);
});
test("le richieste mostrano le storie collegate nella revisione corrente, in attesa e nello storico", () => {
 const change={feedback:'Badge <rosso>',kind:'future-server-kind',story_codes:['US-001','<unsafe>']};
 const ui={incrementSections:{'panel-':true,feedback:true,history:true}};
 for(const increment of [
  {revisions:[{...revision,feedback:change.feedback,change}]},
  {revisions:[revision],pending_change:change},
  {revisions:[{...revision,number:1,feedback:change.feedback,change},revision]}
 ]){
  const html=renderConversation({...base,increment},'',ui);
  assert.match(html,/Storie interessate: US-001, &lt;unsafe&gt;/);
  assert.match(html,/Badge &lt;rosso&gt;/);
  assert.doesNotMatch(html,/<unsafe>|future-server-kind/);
 }
});
test("cronologia ed evidenze sono leggibili e le sezioni aperte resistono al polling", () => {
 const html = renderConversation({...base,increment:{revisions:[{...revision,number:1},revision],result:{summary:"Prova il risultato",checks:["18 verifiche passate"],preview_url:"http://127.0.0.1:3000"},actions:[]}}, "", {incrementSections:{"panel-":true,history:true,checks:true}});
 assert.match(html,/data-increment-section="history" open/);
 assert.match(html,/data-increment-section="checks" open/);
 assert.match(html,/18 verifiche passate/);
 assert.match(html,/Apri anteprima/);
});
test("un turno attivo disabilita i comandi e i contenuti ricevuti non diventano HTML attivo", () => {
 const html = renderConversation({...base,increment:{title:'<img src=x onerror=alert(1)>',revisions:[],busy:true,actions:[{id:'"><img src=x>',label:"Approva"}],result:{summary:"<script>bad()</script>",preview_url:"javascript:bad()"}}}, "", {incrementSections:{"panel-":true}});
 assert.doesNotMatch(html,/<img |<script>|href="javascript:/);
 assert.match(html,/data-increment-action="[^"]*" disabled/);
});
test("il renderer non contiene gli stati o i comandi del processo", () => {
 const source=readFileSync(new URL("../../cli/internal/web/assets/conversation.js",import.meta.url),"utf8");
 for(const token of ["DISCOVERY","DEVELOPING","PUBLISHING","DELIVERED","approve-intent","increment publish","archetipo-increment"]) assert.ok(!source.includes(token),token);
});

test("il contesto è compatto e i dettagli sono chiusi inizialmente", () => {
 const view = {...base, increment:{id:"compact",title:"Incremento",status_label:"Fase fornita",next_steps_label:"Etichetta passi",next_steps_note:"Agente al lavoro · 1/2 storie completate",phases:[{label:"Uno",state:"done"},{label:"Due",state:"current"},{label:"Tre",state:"todo"}],next_steps:[{kind:"open",open:"story",target:"US-002",label:"In lavorazione: US-002 · Filtro"}],revisions:[revision],stories:[{code:"US-002",title:"Filtro",body:"Criteri completi",status_label:"Stato fornito",active:true}],result:{summary:"Risultato molto lungo"}}};
 const compact = renderConversation(view,"bozza",{});
 assert.match(compact,/class="conv-phases"[\s\S]*is-done[^>]*>Uno[\s\S]*aria-current="step">Due[\s\S]*is-todo[^>]*>Tre/);
 assert.match(compact,/data-increment-panel="open"/);
 assert.match(compact,/<form class="conv-composer">[\s\S]*class="conv-increment conv-next"[\s\S]*Etichetta passi[\s\S]*1\/2 storie completate[\s\S]*data-increment-story="US-002"[\s\S]*conv-composer-input/);
 assert.doesNotMatch(compact,/Fase fornita|Risultato molto lungo|Criteri completi|<aside id="increment-details"|class="conv-dir"/);
 const open = renderConversation(view,"bozza",{incrementSections:{"panel-compact":true,"current-story-US-002":true}});
 assert.match(open,/has-increment-open/);
 assert.match(open,/Fase fornita/);
 assert.match(open,/data-increment-section="current-story-US-002" open/);
 assert.match(open,/Stato fornito/);
 assert.match(open,/Criteri completi/);
 assert.match(open,/Risultato molto lungo/);
});
test("i prossimi passi rendono decisioni, prompt e aperture dichiarate dal server", () => {
 const confirm = {title:"Titolo <conferma>",text:"Spiegazione fornita",label:"Sì, procedi"};
 const steps = [
  {kind:"open",open:"conversation",target:"conv-env",tone:"warn",label:"Ambiente fornito"},
  {kind:"decision",action:"azione-attesa",label:"Decisione fornita",confirm},
  {kind:"action",action:"altra-azione",label:"Azione fornita"},
  {kind:"prompt",prompt:"Testo <pronto>: ",label:"Prompt fornito"},
  {kind:"open",open:"url",target:"https://example.test/pr/1",label:"Link fornito"},
  {kind:"open",open:"url",target:"javascript:bad()",label:"Link pericoloso"},
 ];
 const view = {...base, increment:{id:"steps",revisions:[revision],next_steps_label:"Prossimi passi",next_steps:steps}};
 const html = renderConversation(view,"",{});
 assert.match(html,/conv-step conv-step-open is-warn" data-increment-step-open="conversation" data-increment-step-target="conv-env"/);
 assert.match(html,/conv-step-decision" data-increment-step-action="azione-attesa" data-increment-step-confirm="true"/);
 assert.match(html,/conv-step-action" data-increment-step-action="altra-azione">Azione fornita/);
 assert.match(html,/data-increment-step-prompt="Testo &lt;pronto&gt;: "/);
 assert.match(html,/href="https:\/\/example.test\/pr\/1"/);
 assert.doesNotMatch(html,/javascript:|Link pericoloso|data-increment-action=/);
 assert.match(renderConversation({...view,increment:{...view.increment,busy:true}},"",{}),/data-increment-step-action="azione-attesa" data-increment-step-confirm="true" disabled/);
 const armed = renderConversation(view,"la mia bozza",{incrementConfirm:"azione-attesa"});
 assert.match(armed,/class="conv-next-confirm"[\s\S]*Titolo &lt;conferma&gt;[\s\S]*Spiegazione fornita[\s\S]*data-increment-step-cancel[\s\S]*data-increment-step-run="azione-attesa"[^>]*>Sì, procedi/);
 assert.match(armed,/<textarea class="conv-composer-input"[^>]*hidden>la mia bozza<\/textarea>/);
 assert.doesNotMatch(armed,/data-increment-step-action=/);
 assert.doesNotMatch(renderConversation(view,"",{incrementConfirm:"altra-azione"}),/conv-next-confirm/);
});
test("l’avviso sulle integrazioni simulate compare solo quando il server lo fornisce, accanto al risultato", () => {
 const result = {summary:"Prova il risultato",checks:["18 verifiche passate"]};
 const real = renderConversation({...base,increment:{id:"inc-1",revisions:[revision],result,actions:[{id:"publish",label:"Approva"}]}}, "", {incrementSections:{"panel-inc-1":true}});
 assert.doesNotMatch(real,/simulat/i);
 const simulated = renderConversation({...base,increment:{id:"inc-1",revisions:[revision],result,result_notice:"Provato con integrazioni simulate: <google>. La prova reale avviene dopo il merge.",actions:[{id:"publish",label:"Approva"}]}}, "", {incrementSections:{"panel-inc-1":true}});
 assert.match(simulated,/conv-increment-result[\s\S]*Provato con integrazioni simulate: &lt;google&gt;[\s\S]*data-increment-action="publish"/);
 assert.doesNotMatch(simulated,/<google>/);
});
