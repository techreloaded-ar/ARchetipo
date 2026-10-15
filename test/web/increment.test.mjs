import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
import { test } from "node:test";

const require = createRequire(import.meta.url);
const { renderConversation } = require("../../cli/internal/web/assets/conversation.js");
const base = { available: true, conversation: { id: "conv-1", state: "ACTIVE" }, events: [] };
const revision = { number: 2, intent: "Un risultato concordato", approved: false, stories: [{code:"US-001",title:"Esportare i clienti",body:"Criteri leggibili"}] };

test("l’incremento usa le azioni e lo stato del server, preservando il compositore libero", () => {
 const html = renderConversation({...base, increment:{id:"inc-1",title:"Esportazione",request:"La richiesta",status_label:"Etichetta fornita",note:"Nota fornita",revisions:[revision],actions:[{id:"azione-mai-vista",label:"Comando fornito"}]}}, "la mia bozza", {});
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
test("cronologia ed evidenze sono leggibili e le sezioni aperte resistono al polling", () => {
 const html = renderConversation({...base,increment:{revisions:[{...revision,number:1},revision],result:{summary:"Prova il risultato",checks:["18 verifiche passate"],preview_url:"http://127.0.0.1:3000"},actions:[]}}, "", {incrementSections:{history:true,checks:true}});
 assert.match(html,/data-increment-section="history" open/);
 assert.match(html,/data-increment-section="checks" open/);
 assert.match(html,/18 verifiche passate/);
 assert.match(html,/Apri anteprima/);
});
test("un turno attivo disabilita i comandi e i contenuti ricevuti non diventano HTML attivo", () => {
 const html = renderConversation({...base,increment:{title:'<img src=x onerror=alert(1)>',revisions:[],busy:true,actions:[{id:'"><img src=x>',label:"Approva"}],result:{summary:"<script>bad()</script>",preview_url:"javascript:bad()"}}}, "", {});
 assert.doesNotMatch(html,/<img |<script>|href="javascript:/);
 assert.match(html,/data-increment-action="[^"]*" disabled/);
});
test("il renderer non contiene gli stati o i comandi del processo", () => {
 const source=readFileSync(new URL("../../cli/internal/web/assets/conversation.js",import.meta.url),"utf8");
 for(const token of ["DISCOVERY","DEVELOPING","PUBLISHING","DELIVERED","approve-intent","increment publish","archetipo-increment"]) assert.ok(!source.includes(token),token);
});
