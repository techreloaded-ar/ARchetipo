// DOM controller. The frame lives outside the conversation renderer so polling
// the chat never reloads the application or loses a page being inspected.
(function () {
 "use strict";
 const host=document.getElementById("browser-preview");
 if(!host || !window.Preview) return;
 host.innerHTML=`<header class="preview-head"><strong>Preview</strong><button type="button" data-preview="close" aria-label="Chiudi preview">Chiudi</button></header>
 <div class="preview-tools"><button type="button" data-preview="start">Avvia preview</button><button type="button" data-preview="setup">Prepara con l’agente</button><button type="button" data-preview="stop">Arresta</button><button type="button" data-preview="reload">Ricarica</button><a data-preview="external" target="_blank" rel="noopener noreferrer">Apri fuori ↗</a><label>Schermo <select data-preview="width"><option value="100%">Desktop</option><option value="820px">Tablet</option><option value="390px">Mobile</option></select></label><button type="button" data-preview="pick">Commenta un elemento</button></div>
 <p class="preview-status" role="status" aria-live="polite"></p><details class="preview-log"><summary>Output del server</summary><pre></pre></details>
 <div class="preview-stage"></div><section class="preview-comments" aria-label="Commenti da inviare"><div class="preview-comment-list"></div><button type="button" data-preview="send">Invia modifiche</button></section>`;
 host.querySelectorAll("button").forEach(button=>button.classList.add("ghost-btn"));
 host.querySelector("[data-preview=send]").className="primary-btn";
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
  find("pick").textContent=picking?"Seleziona un elemento · Esc per annullare":"Commenta un elemento";
  find("pick").setAttribute("aria-pressed",String(picking));
  find("setup").disabled=busy || !current?.writable;
  find("start").disabled=busy || !view?.configured || ["starting","ready"].includes(view?.state);
  find("stop").disabled=busy || !["starting","ready","error"].includes(view?.state);
  find("reload").disabled=!frame;
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
  document.querySelector(".workspace-primary")?.classList.toggle("has-browser-preview",open && !!current);
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
   open=saved.open;comments=saved.comments;drawComments();visibility();
  },
  toggle(){if(!current)return;open=!open;persist();visibility();}
 };
 host.addEventListener("click",async event=>{
  const button=event.target.closest("[data-preview]");if(!button || button.disabled)return;
  const action=button.dataset.preview;
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
