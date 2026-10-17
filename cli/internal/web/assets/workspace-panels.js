// Persistent workspace chrome. Neither chat polling nor tab changes remount the
// preview iframe. Only the details slot is refreshed by the conversation renderer.
(function(){
 "use strict";
 const shell=document.getElementById("workspace-shell");
 const primary=document.getElementById("workspace-primary");
 const inspector=document.getElementById("workspace-inspector");
 const details=document.getElementById("inspector-details");
 const preview=document.getElementById("browser-preview");
 if(!shell || !inspector || !window.WorkspaceColumns)return;
 let pref;
 try{pref=window.WorkspaceColumns.preferences(JSON.parse(localStorage.getItem("archetipo-columns")||"{}"));}catch{pref=window.WorkspaceColumns.preferences();}
 const railToggle=document.getElementById("rail-toggle");
 const railSplitter=document.getElementById("rail-splitter");
 const splitter=document.getElementById("inspector-splitter");
 const states=new Map();
 let key="",selected="preview",opened=false,expanded=false,hasDetails=false,detailsLabel="Storie";
 function save(){try{localStorage.setItem("archetipo-columns",JSON.stringify(pref));}catch{/* Layout still works without storage. */}}
 function applyWidths(){
  shell.style.setProperty("--rail-width",(pref.collapsed?44:pref.rail)+"px");
  shell.classList.toggle("rail-collapsed",pref.collapsed);
  railToggle.setAttribute("aria-expanded",String(!pref.collapsed));
  railToggle.setAttribute("aria-label",pref.collapsed?"Apri elenco conversazioni":"Riduci elenco conversazioni");
  railToggle.title=pref.collapsed?"Apri elenco conversazioni":"Riduci elenco conversazioni";
  railToggle.querySelector("svg").style.transform=pref.collapsed?"rotate(180deg)":"";
  railSplitter.setAttribute("aria-valuenow",String(Math.round(pref.rail)));
  const width=window.WorkspaceColumns.inspector(primary.clientWidth,pref.ratio);
  primary.style.setProperty("--inspector-width",width+"px");
  splitter.setAttribute("aria-valuenow",String(width));
  splitter.setAttribute("aria-valuemax",String(Math.max(300,primary.clientWidth-300)));
 }
 function remember(){if(key)states.set(key,{selected,opened,expanded});}
 const expandButton=inspector.querySelector("[data-inspector-expand]");
 const shortcut=/Mac|iPhone|iPad/.test(navigator.platform)?"⌘⌥B":"Ctrl+Alt+B";
 function syncControls(){
  for(const button of shell.querySelectorAll("[data-work-panel-toggle]")){
   const label=opened?"Chiudi pannello":"Apri pannello";
   button.setAttribute("aria-expanded",String(opened));
   button.setAttribute("aria-label",label);
   button.setAttribute("aria-keyshortcuts","Control+Alt+B Meta+Alt+B");
   button.title=label+" · "+shortcut;
  }
 }
 function draw(){
  inspector.hidden=!opened;
  splitter.hidden=!opened || expanded;
  primary.classList.toggle("inspector-expanded",opened && expanded);
  const conversation=document.getElementById("workspace-conversation");
  if(conversation)conversation.inert=opened && expanded;
  expandButton.setAttribute("aria-pressed",String(expanded));
  expandButton.setAttribute("aria-label",expanded?"Ripristina pannello":"Espandi pannello");
  expandButton.title=expanded?"Ripristina pannello · Esc":"Espandi pannello";
  expandButton.querySelector("path").setAttribute("d",expanded?"M3 8h5V3m8 0v5h5M8 21v-5H3m18 0h-5v5":"M8 3H3v5m13-5h5v5M3 16v5h5m13-5v5h-5");
  syncControls();
  primary.classList.toggle("has-inspector",opened);
  for(const button of inspector.querySelectorAll("[data-inspector-tab]")){
   const isDetails=button.dataset.inspectorTab==="details";
   button.disabled=isDetails&&!hasDetails;
   button.setAttribute("aria-selected",String(button.dataset.inspectorTab===selected));
   button.tabIndex=button.dataset.inspectorTab===selected?0:-1;
   if(isDetails)button.textContent=detailsLabel;
  }
  details.hidden=selected!=="details";
  preview.hidden=selected!=="preview";
  window.BrowserPreview?.setActive(opened && selected==="preview");
  applyWidths();
 }
 function open(tab){
  if(!key || (tab==="details"&&!hasDetails))return;
  selected=tab;opened=true;remember();draw();
  if(tab==="details")window.dispatchEvent(new CustomEvent("archetipo:details-open"));
 }
 function close(){
  const returnFocus=inspector.contains(document.activeElement);
  opened=false;expanded=false;remember();draw();
  window.dispatchEvent(new CustomEvent("archetipo:details-close"));
  if(returnFocus)shell.querySelector("[data-work-panel-toggle]")?.focus({preventScroll:true});
 }
 window.WorkspacePanels={
  configure(value){
   if(key!==value.key){remember();key=value.key||"";const state=states.get(key);selected=state?.selected||"preview";opened=state?.opened||false;expanded=state?.expanded||false;}
   hasDetails=value.hasDetails===true;detailsLabel=value.detailsLabel||"Storie";
   if(!hasDetails&&selected==="details")selected="preview";
   if(!key)opened=false;
   draw();
  },
  open,close,
  reset(){opened=false;expanded=false;remember();draw();},
  toggle(tab){if(opened&&selected===tab)close();else open(tab);},
  detailsSelected(){return opened && selected==="details";},
  isPreviewActive(){return opened && selected==="preview";},
  attach(nodes){details.replaceChildren(...nodes.filter(Boolean));syncControls();},
 };
 function togglePanel(){
  if(opened)close();
  else{open(selected);inspector.querySelector('[aria-selected="true"]')?.focus({preventScroll:true});}
 }
 shell.addEventListener("click",event=>{if(event.target.closest("[data-work-panel-toggle]"))togglePanel();});
 document.addEventListener("keydown",event=>{
  if(event.defaultPrevented || event.isComposing || !key || shell.dataset.view!=="conversation")return;
  const modal=document.querySelector('[role="dialog"][aria-modal="true"]:not(.hidden):not([hidden])');
  if(modal)return;
  if(event.code==="KeyB" && event.altKey && (event.metaKey || event.ctrlKey) && !event.shiftKey){event.preventDefault();togglePanel();return;}
  if(event.key!=="Escape" || !opened || document.fullscreenElement)return;
  const menu=inspector.querySelector(".preview-menu[open]");
  if(menu){event.preventDefault();event.stopPropagation();menu.open=false;menu.querySelector("summary")?.focus();return;}
  if(shell.querySelector('[data-conversation-pop]:not([hidden])'))return;
  if(expanded){event.preventDefault();event.stopPropagation();expanded=false;remember();draw();expandButton.focus({preventScroll:true});}
 },true);
 railToggle.addEventListener("click",()=>{pref.collapsed=!pref.collapsed;applyWidths();save();});
 inspector.addEventListener("click",event=>{
  const tab=event.target.closest("[data-inspector-tab]");
  if(tab)open(tab.dataset.inspectorTab);
  if(event.target.closest("[data-inspector-expand]")){expanded=!expanded;remember();draw();}
  if(event.target.closest("[data-inspector-close]"))close();
 });
 inspector.querySelector('[role="tablist"]').addEventListener("keydown",event=>{
  if(!["ArrowLeft","ArrowRight","Home","End"].includes(event.key))return;
  event.preventDefault();const tab=selected==="preview"&&hasDetails?"details":"preview";open(tab);
  inspector.querySelector(`[data-inspector-tab="${tab}"]`).focus();
 });
 shell.addEventListener("keydown",event=>{
  if(event.key==="Escape" && opened && !event.defaultPrevented && inspector.contains(event.target) && !event.target.closest(".preview-tools, .preview-comment, .preview-menu[open]")){close();}
 });
 function resize(handle,kind){
  const change=(dx,start)=>{
   if(kind==="rail"){pref.rail=window.WorkspaceColumns.clamp(start.rail+dx,190,420);pref.collapsed=false;}
   else{const width=primary.clientWidth;pref.ratio=window.WorkspaceColumns.clamp((window.WorkspaceColumns.inspector(width,start.ratio)-dx)/width,.28,.68);}
   applyWidths();
  };
  handle.addEventListener("pointerdown",event=>{
   if(event.button!==0)return;event.preventDefault();
   const start={...pref},x=event.clientX;
   handle.setPointerCapture(event.pointerId);document.body.classList.add("resizing-columns");
   const move=next=>change(next.clientX-x,start);
   const finish=()=>{handle.removeEventListener("pointermove",move);handle.removeEventListener("pointerup",finish);handle.removeEventListener("pointercancel",finish);document.body.classList.remove("resizing-columns");save();};
   handle.addEventListener("pointermove",move);handle.addEventListener("pointerup",finish);handle.addEventListener("pointercancel",finish);
  });
  handle.addEventListener("keydown",event=>{
   if(!["ArrowLeft","ArrowRight","Home","End"].includes(event.key))return;
   event.preventDefault();
   if(event.key==="Home"){if(kind==="rail")pref.rail=190;else pref.ratio=.28;}
   else if(event.key==="End"){if(kind==="rail")pref.rail=420;else pref.ratio=.68;}
   else change(event.key==="ArrowRight"?16:-16,{...pref});
   applyWidths();save();
  });
  handle.addEventListener("dblclick",()=>{if(kind==="rail")pref.rail=264;else pref.ratio=.48;applyWidths();save();});
 }
 resize(railSplitter,"rail");resize(splitter,"inspector");
 new ResizeObserver(applyWidths).observe(primary);
 applyWidths();
})();
