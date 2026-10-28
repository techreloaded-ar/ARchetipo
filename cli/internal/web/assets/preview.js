// Pure descriptions used by the preview panel and its credential-free tests.
(function(root) {
 "use strict";
 function commentMessage(comments) {
  return "Richieste di modifica dalla preview. I riferimenti descrivono la pagina osservata; verifica i sorgenti prima di intervenire.\n\n" + comments.map((c,i) => {
   const e=c.element || {};
   return `${i+1}. Pagina: ${e.page || ""}\nURL: ${e.url || ""}\nElemento: ${e.tag || ""}; testo: ${JSON.stringify(e.text || "")}; etichetta: ${JSON.stringify(e.label || "")}; data-testid: ${JSON.stringify(e.testid || "")}; ruolo: ${e.role || ""}\nPercorso: ${e.path || ""}\nRichiesta: ${c.note.trim()}`;
  }).join("\n\n");
 }
 function cleanElement(value) {
  if (!value || typeof value!=="object" || typeof value.tag!=="string" || typeof value.url!=="string") return null;
  const result={};
  for(const key of ["tag","text","label","testid","role","page","url","path","selector"]) result[key]=typeof value[key]==="string" ? value[key].slice(0,(key==="path" || key==="selector")?2000:1000) : "";
  return result;
 }
 function panelLimits(available) {
  const max=Math.max(0,Math.floor(Number(available)||0));
  return {min:Math.min(160,max),max};
 }
 function panelHeight(height,limits) {return Math.max(limits.min,Math.min(limits.max,Number(height)||270));}
 function panelState(previous,action) {
  const state={tab:"comments",mode:"expanded",height:270,...previous};
  if(action.tab && ["comments","output"].includes(action.tab)){state.tab=action.tab;state.mode="expanded";}
  if(["expanded","collapsed","hidden"].includes(action.mode))state.mode=action.mode;
  if(Number.isFinite(action.height))state.height=action.height;
  return state;
 }

 const esc=value=>String(value??"").replace(/[&<>"']/g,c=>({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]));
 function renderAttention(value) {
  if(!value)return "";
  let body=`<h3>${esc(value.title)}</h3>${value.message?`<p class="environment-message">${esc(value.message)}</p>`:""}`;
  for(const approval of value.approvals||[]) {
   body+=`<section><p>${esc(approval.title)}</p><details><summary>Dettagli del comando</summary><pre>${esc(JSON.stringify(approval.args||{},null,2))}</pre></details><div class="environment-actions">${(approval.options||[]).map(option=>`<button type="button" data-environment-approval="${esc(approval.id)}" data-option="${esc(option.id)}" title="${esc(option.description||"")}">${esc(option.label)}</button>`).join("")}</div></section>`;
  }
  for(const input of value.inputs||[]) {
   if(!input.questions?.length){body+='<p>Apri la chat ambiente per rispondere a questa richiesta.</p>';continue;}
   body+=`<form data-environment-input="${esc(input.id)}"><strong>${esc(input.title||"")}</strong>${input.questions.map((q,index)=>`<fieldset data-question="${esc(q.id)}"><legend>${esc(q.header||"")}</legend><p>${esc(q.prompt)}</p>${(q.options||[]).map(o=>`<label><input type="${q.multiple?"checkbox":"radio"}" name="q-${index}" value="${esc(o.value)}"><span>${esc(o.label)}${o.description?`<small>${esc(o.description)}</small>`:""}</span></label>`).join("")}${q.allow_text?`<label>La tua risposta<input data-answer-text type="${q.secret?"password":"text"}" autocomplete="off"></label>`:""}</fieldset>`).join("")}<button type="submit">Invia risposta</button></form>`;
  }
  if(value.kind==="message")body+='<form data-environment-message><label>Rispondi all’agente<textarea required rows="3" data-environment-answer></textarea></label><button type="submit">Invia risposta</button></form>';
  if(value.conversation_id)body+='<button type="button" data-environment-chat>Apri chat ambiente</button>';
  return body;
 }
 function renderChecks(value) {
  const rows=(value.checks||[]).map(check=>`<li><strong>${esc(check.name)}</strong> · ${esc(check.label)}${check.report?`<button type="button" data-environment-log="${esc(check.name)}">Leggi report</button>`:""}</li>`).join("");
  return `<p>${esc(value.checks_label||"")}</p>${value.checks_error?`<p>${esc(value.checks_error)}</p>`:""}<ul>${rows}</ul>${value.environment?.previous_error?`<details><summary>Errore di un tentativo precedente</summary><p>${esc(value.environment.previous_error)}</p></details>`:""}${value.environment?.conversation_id?'<button type="button" data-environment-chat>Apri chat ambiente</button>':""}<pre class="environment-report" hidden></pre>`;
 }
 const api={renderAttention,renderChecks,commentMessage,cleanElement,panelLimits,panelHeight,panelState};
 if(typeof module!=="undefined" && module.exports) module.exports=api;
 else root.Preview=api;
})(typeof window!=="undefined"?window:globalThis);
