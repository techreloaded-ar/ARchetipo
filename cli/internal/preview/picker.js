function (config) {
 "use strict";
 if (parent === window) return;
 let active = false, highlighted = null, animation = 0;
 const overlay = document.createElement("div");
 overlay.dataset.archetipoHighlight = "";
 overlay.style.cssText = "position:fixed;pointer-events:none;z-index:2147483647;border:2px solid #7064d9;background:#7064d91a;display:none;box-sizing:border-box;border-radius:4px";
 const badge = document.createElement("span");
 badge.style.cssText = "position:absolute;top:0;left:0;padding:2px 6px;background:#7064d9;color:white;font:600 12px/18px system-ui;border-radius:4px";
 badge.hidden=true;overlay.append(badge);
 document.documentElement.appendChild(overlay);
 const emit = (type, element) => parent.postMessage({type, token:config.token, element}, config.origin);
 const paint = el => {
  if(!el?.isConnected){overlay.style.display="none";return;}
  const r=el.getBoundingClientRect();
  badge.style.top=r.top>=24?"-24px":"0";
  Object.assign(overlay.style,{display:"block",left:r.left+"px",top:r.top+"px",width:r.width+"px",height:r.height+"px"});
 };
 const clearHighlight = () => {highlighted=null;cancelAnimationFrame(animation);overlay.style.display="none";badge.hidden=true;};
 const track = () => {paint(highlighted);if(highlighted?.isConnected)animation=requestAnimationFrame(track);};
 const stop = () => {active=false;clearHighlight();};
 const describe = (el) => {
  const path=[], selector=[];
  for(let p=el;p && p!==document.body;p=p.parentElement) {
   const nth=":nth-of-type("+(Array.from(p.parentElement?.children || []).filter(s=>s.tagName===p.tagName).indexOf(p)+1)+")";
   if(path.length<6){
    const id=p.getAttribute("data-testid") || p.id;
    path.unshift(p.tagName.toLowerCase()+(id ? "["+id+"]" : nth));
   }
   selector.unshift(p.tagName.toLowerCase()+nth);
  }
  return {tag:el.tagName.toLowerCase(),text:(el.textContent || "").replace(/\s+/g," ").trim().slice(0,300),label:el.getAttribute("aria-label") || el.getAttribute("title") || el.getAttribute("placeholder") || "",testid:el.closest("[data-testid]")?.getAttribute("data-testid") || "",role:el.getAttribute("role") || "",page:document.title,url:location.href,path:path.join(" > "),selector:el===document.body?"body":"body > "+selector.join(" > ")};
 };
 // Keep exact live references; the selector restores the association after reload.
 const targets=new Map();
 function resolve(element){
  const known=targets.get(element.selector || element.path);
  if(known?.isConnected)return known;
  let candidates=[];
  try{
   if(element.selector)candidates=Array.from(document.querySelectorAll(element.selector));
   else candidates=Array.from(document.getElementsByTagName(element.tag));
  }catch{return null;}
  candidates=candidates.filter(el=>{
   const value=describe(el);
   return value.tag===element.tag && value.text===element.text && value.label===element.label && value.testid===element.testid && (element.selector || value.path===element.path);
  });
  return candidates.length===1?candidates[0]:null;
 }
 document.addEventListener("pointermove",e=>{
  if(active && e.target instanceof Element)paint(e.target);
 },true);
 document.addEventListener("click",e=>{
  if(!active || !(e.target instanceof Element)) return;
  e.preventDefault(); e.stopImmediatePropagation();
  const element=describe(e.target);targets.set(element.selector,e.target);
  emit("archetipo-picked",element); stop();
 },true);
 document.addEventListener("keydown",e=>{if(active && e.key==="Escape"){e.preventDefault();stop();emit("archetipo-picker-cancelled");}},true);
 addEventListener("message",e=>{
  if(e.source!==parent || e.origin!==config.origin || e.data?.token!==config.token) return;
  if(e.data.type==="archetipo-picker-hello") emit("archetipo-picker-ready");
  if(e.data.type==="archetipo-picker-toggle"){stop();active=Boolean(e.data.active);}
  if(e.data.type==="archetipo-comment-highlight"){
   clearHighlight();
   const element=e.data.element;
   if(active || !element)return;
   let samePage=false;
   try{const url=new URL(element.url,location.href);samePage=url.pathname===location.pathname && url.search===location.search;}catch{/* Invalid references cannot highlight the current page. */}
   const target=samePage?resolve(element):null;
   parent.postMessage({type:"archetipo-comment-highlighted",token:config.token,number:e.data.number,found:!!target,reason:samePage?"missing":"page"},config.origin);
   if(!target)return;
   highlighted=target;badge.textContent=String(e.data.number);badge.hidden=false;
   target.scrollIntoView({block:"nearest",inline:"nearest",behavior:"instant"});track();
  }
 });
 emit("archetipo-picker-ready");
}
