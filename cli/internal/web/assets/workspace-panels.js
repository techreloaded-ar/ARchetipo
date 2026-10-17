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
 let key="",selected="preview",opened=false,hasDetails=false,detailsLabel="Storie";
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
 function remember(){if(key)states.set(key,{selected,opened});}
 function draw(){
  inspector.hidden=!opened;
  splitter.hidden=!opened;
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
  opened=false;remember();draw();
  window.dispatchEvent(new CustomEvent("archetipo:details-close"));
 }
 window.WorkspacePanels={
  configure(value){
   if(key!==value.key){remember();key=value.key||"";const state=states.get(key);selected=state?.selected||"preview";opened=state?.opened||false;}
   hasDetails=value.hasDetails===true;detailsLabel=value.detailsLabel||"Storie";
   if(!hasDetails&&selected==="details")selected="preview";
   if(!key)opened=false;
   draw();
  },
  open,close,
  reset(){opened=false;remember();draw();},
  toggle(tab){if(opened&&selected===tab)close();else open(tab);},
  detailsSelected(){return opened && selected==="details";},
  isPreviewActive(){return opened && selected==="preview";},
  attach(nodes){details.replaceChildren(...nodes.filter(Boolean));},
 };
 railToggle.addEventListener("click",()=>{pref.collapsed=!pref.collapsed;applyWidths();save();});
 inspector.addEventListener("click",event=>{
  const tab=event.target.closest("[data-inspector-tab]");
  if(tab)open(tab.dataset.inspectorTab);
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
