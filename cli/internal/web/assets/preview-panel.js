// DOM controller. The frame lives outside the conversation renderer so polling
// the chat never reloads the application or loses a page being inspected.
(function () {
 "use strict";
 const host=document.getElementById("browser-preview");
 if(!host || !window.Preview) return;
 const icon=(path)=>`<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${path}</svg>`;
 const play=icon('<path d="m9 5 11 7-11 7Z"/>');
 const refreshIcon=icon('<path d="M20 7v5h-5M4 17a8 8 0 0 0 14 1M20 12A8 8 0 0 0 6 6L4 9"/>');
 const pointer=icon('<path d="m5 3 14 10-7 1-3 7Z"/>');
 const external=icon('<path d="M14 3h7v7M21 3 10 14M10 3H4v17h17v-6"/>');
 const monitor=icon('<rect x="3" y="4" width="18" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/>');
 host.innerHTML=`<div class="preview-toolbar">
  <div class="preview-location"><span class="preview-state-dot"></span><span class="preview-location-text">Anteprima locale</span></div>
  <div class="preview-toolbar-actions"><button type="button" class="preview-launch" data-preview="setup">${play}<span>Prepara preview</span></button><button type="button" class="preview-launch" data-preview="start">${play}<span>Avvia preview</span></button><button type="button" class="preview-icon" data-preview="reload" aria-label="Ricarica preview" title="Ricarica">${refreshIcon}</button><a class="preview-icon" data-preview="external" target="_blank" rel="noopener noreferrer" aria-label="Apri preview nel browser" title="Apri nel browser">${external}</a><details class="preview-menu"><summary class="preview-icon" aria-label="Altre opzioni preview" title="Altre opzioni">···</summary><div><button type="button" data-preview="reconfigure">Prepara di nuovo con l’agente</button><button type="button" class="preview-stop" data-preview="stop">Arresta server</button></div></details></div>
 </div>
 <div class="preview-tools"><label class="preview-viewport">${monitor}<span class="sr-only">Dimensioni schermo</span><select data-preview="width" aria-label="Dimensioni schermo"><option value="100%">Adatta</option><option value="820px">Tablet · 820</option><option value="390px">Mobile · 390</option></select></label><button type="button" class="preview-pick" data-preview="pick">${pointer}<span>Commenta un elemento</span></button></div>
 <div class="preview-stage"><div class="preview-placeholder"><svg width="64" height="54" viewBox="0 0 64 54" fill="none" aria-hidden="true"><rect x="5" y="5" width="54" height="43" rx="7" stroke="currentColor" stroke-width="1.4"/><path d="M5 17h54M13 11h1m4 0h1m4 0h1M25 27l-5 5 5 5m14-10 5 5-5 5" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"/></svg><h3>Il tuo lavoro, qui.</h3><p>Avvia l’applicazione per esplorare il risultato e indicare cosa cambiare.</p></div></div>
 <p class="preview-status" role="status" aria-live="polite"></p><details class="preview-log"><summary>Output del server</summary><pre></pre></details>
 <section class="preview-comments" aria-label="Commenti da inviare"><div class="preview-comments-head"><strong>Modifiche da chiedere</strong><span class="preview-comments-count"></span></div><div class="preview-comment-list"></div><div class="preview-comments-footer"><button type="button" class="preview-launch" data-preview="send">Invia modifiche</button></div></section>`;
 const find=k=>host.querySelector(`[data-preview="${k}"]`);
 const stage=host.querySelector(".preview-stage"), status=host.querySelector(".preview-status"), list=host.querySelector(".preview-comment-list");
 let current=null, key="", open=false, frame=null, view=null, ready=false, picking=false, busy=false, poll=null, request=0;
 let comments=[], notice="";
 const states=new Map();
 const route=()=>`/api/workspace/conversations/${encodeURIComponent(current.id)}/preview`;
 async function api(url,method="GET") {
  const response=await fetch(url,{method,headers:method==="GET"?{}:{"Content-Type":"application/json"},body:method==="POST"?"{}":undefined});
  const value=await response.json();if(!response.ok)throw new Error(value.error || value.message || "Preview non disponibile");return value;
 }
 function persist(){
  if(!key)return;
  states.set(key,{open,comments});
  try{localStorage.setItem(key,JSON.stringify(comments));}catch{/* A restricted browser can retain drafts in memory. */}
 }
 function showError(error){notice=error.message || String(error);status.textContent=notice;}
 function messageToAgent(message){return new Promise((resolve,reject)=>window.dispatchEvent(new CustomEvent("archetipo:preview-send",{detail:{id:current.id,message,resolve,reject}})));}
 function post(type,fields={}){if(frame && view)frame.contentWindow?.postMessage({type,token:view.token,...fields},new URL(view.url).origin);}
 function clearFrame(){frame?.remove();frame=null;ready=false;picking=false;}
 function drawComments(){
  list.replaceChildren();
  comments.forEach((comment,index)=>{
   const row=document.createElement("div");row.className="preview-comment";
   const label=document.createElement("label");label.textContent=`${index+1}. ${comment.element.label || comment.element.text || comment.element.tag}`;
   const input=document.createElement("textarea");input.rows=2;input.value=comment.note;input.placeholder="Cosa vuoi cambiare?";input.maxLength=4000;input.setAttribute("aria-label",`Modifica ${index+1}`);
   input.addEventListener("input",()=>{comment.note=input.value;persist();updateButtons();});
   const remove=document.createElement("button");remove.className="ghost-btn";remove.type="button";remove.textContent="Rimuovi";remove.addEventListener("click",()=>{comments.splice(index,1);persist();drawComments();});
   label.append(input);row.append(label,remove);list.append(row);
  });
  updateButtons();
 }
 function updateButtons(){
  list.querySelectorAll("textarea,button").forEach(node=>{node.disabled=busy;});
  find("send").disabled=busy || !current?.writable || !comments.length || comments.some(c=>!c.note.trim());
  find("send").textContent=`Invia modifiche${comments.length ? " ("+comments.length+")" : ""}`;
  find("pick").disabled=!ready || busy || comments.length>=30;
  find("pick").querySelector("span").textContent=picking?"Selezione attiva · Esc":"Commenta un elemento";
  find("pick").setAttribute("aria-pressed",String(picking));
  find("setup").disabled=busy || !current?.writable;
  find("start").disabled=busy || !view?.configured || ["starting","ready"].includes(view?.state);
  find("stop").disabled=busy || !["starting","ready","error"].includes(view?.state);
  find("reload").disabled=!frame;
  const state=view?.state || "unconfigured";
  host.dataset.state=state;
  find("setup").hidden=state!=="unconfigured";
  find("start").hidden=["unconfigured","ready"].includes(state);
  find("start").querySelector("span").textContent=state==="starting"?"Avvio in corso…":"Avvia preview";
  find("pick").hidden=state!=="ready";
  find("reload").hidden=state!=="ready";
  find("external").hidden=state!=="ready";
  host.querySelector(".preview-tools").hidden=state!=="ready";
  host.querySelector(".preview-comments").hidden=!comments.length;
  host.querySelector(".preview-comments-count").textContent=String(comments.length);
  host.querySelector(".preview-placeholder").hidden=!!frame;
  host.querySelector(".preview-location-text").textContent=view?.recipe?.url || "Anteprima locale";
  host.querySelector(".preview-location-text").title=view?.recipe?.url || "";
 }
 function apply(value){
  view=value;
  host.querySelector(".preview-log pre").textContent=value.log || "";
  host.querySelector(".preview-log").hidden=!value.log;
  const labels={unconfigured:"Chiedi all’agente di preparare la preview del progetto.",stopped:"Preview pronta da avviare.",starting:"Avvio del server in corso…",ready:"Preview disponibile.",error:value.error};
  status.textContent=notice || labels[value.state] || value.state;
  if(value.state==="ready") {
   find("external").href=value.url;
   if(!frame || frame.dataset.previewURL!==value.url){
    clearFrame();frame=document.createElement("iframe");frame.title="Anteprima del lavoro";frame.dataset.previewURL=value.url;
    frame.setAttribute("sandbox","allow-scripts allow-same-origin allow-forms allow-modals allow-downloads");
    frame.style.width=find("width").value;
    frame.addEventListener("load",()=>{status.textContent="Preview disponibile. Se il selettore non si attiva, usa la conversazione per un commento generale.";ready=false;picking=false;post("archetipo-picker-hello");updateButtons();});
    frame.src=value.url;stage.append(frame);
   }
   post("archetipo-picker-hello");
  } else {clearFrame();find("external").removeAttribute("href");}
  updateButtons();
 }
 async function refresh(){
  if(!current || !open || busy)return;
  const generation=++request,url=route();
  try{const value=await api(url);if(generation===request)apply(value);}catch(error){if(generation===request)showError(error);}
 }
 function visibility(){
  host.hidden=!open || !current;

  clearInterval(poll);poll=null;
  if(open && current){void refresh();poll=setInterval(refresh,1500);}
 }
 window.BrowserPreview={
  select(value){
   const next=value?"archetipo-preview:"+value.root+":"+value.id:"";
   if(key===next){current=value;updateButtons();return;}
   persist();request++;clearFrame();current=value;key=next;view=null;ready=false;busy=false;notice="";
   let saved=states.get(key);
   if(!saved){let drafts=[];try{drafts=JSON.parse(localStorage.getItem(key)||"[]");}catch{/* Invalid drafts are ignored. */}saved={open:false,comments:Array.isArray(drafts)?drafts.filter(c=>window.Preview.cleanElement(c?.element) && typeof c.note==="string").slice(0,30):[]};}
   open=window.WorkspacePanels?.isPreviewActive() || false;comments=saved.comments;drawComments();visibility();
  },
  setActive(active){if(open===active)return;open=active;persist();visibility();},
  toggle(){if(!current)return;if(window.WorkspacePanels){window.WorkspacePanels.toggle("preview");return;}open=!open;persist();visibility();}
 };
 host.addEventListener("click",async event=>{
  const button=event.target.closest("[data-preview]");if(!button || button.disabled)return;
  const action=button.dataset.preview==="reconfigure"?"setup":button.dataset.preview;
  host.querySelector(".preview-menu").open=false;
  if(action==="close"){open=false;persist();visibility();return;}
  if(action==="pick"){picking=!picking;post("archetipo-picker-toggle",{active:picking});updateButtons();return;}
  if(action==="reload"){if(frame){ready=false;frame.src=frame.src;}return;}
  if(!["start","stop","setup","send"].includes(action) || busy)return;
  const selected=key, url=route();request++;notice="";busy=true;updateButtons();
  try{
   if(action==="setup"){await messageToAgent(window.Preview.setupMessage());if(key===selected){notice="Ho chiesto all’agente di preparare la preview.";status.textContent=notice;}}
   if(action==="send"){
    const batch=comments;
    await messageToAgent(window.Preview.commentMessage(batch));
    // Clear the submitted batch only; a different conversation keeps its drafts.
    const saved=states.get(selected);if(saved){saved.comments=[];}
    try{localStorage.removeItem(selected);}catch{/* Memory state remains authoritative. */}
    if(key===selected){comments=[];drawComments();persist();notice="Richieste inviate alla conversazione.";status.textContent=notice;}
   }
   if(action==="start" || action==="stop"){
    const value=await api(url,action==="start"?"POST":"DELETE");if(key===selected)apply(value);
   }
  }catch(error){if(key===selected)showError(error);}
  finally{if(key===selected){busy=false;updateButtons();}}
 });
 find("width").addEventListener("change",()=>{if(frame)frame.style.width=find("width").value;});
 addEventListener("message",event=>{
  if(!frame || !view || event.source!==frame.contentWindow || event.origin!==new URL(view.url).origin || event.data?.token!==view.token)return;
  if(event.data.type==="archetipo-picker-ready"){ready=true;status.textContent=notice || "Preview disponibile. Puoi navigare o commentare un elemento.";}
  if(event.data.type==="archetipo-picker-cancelled")picking=false;
  if(event.data.type==="archetipo-picked"){
   picking=false;const element=window.Preview.cleanElement(event.data.element);
   if(element && comments.length<30 && !busy){
    try{const location=new URL(element.url);location.host=new URL(view.recipe.url).host;element.url=location.href;}catch{/* Keep the observed URL if it cannot be mapped. */}comments.push({element,note:""});persist();drawComments();list.lastElementChild?.querySelector("textarea")?.focus();}
  }
  updateButtons();
 });
})();
