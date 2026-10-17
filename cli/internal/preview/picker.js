function (config) {
 "use strict";
 if (parent === window) return;
 let active = false;
 const overlay = document.createElement("div");
 overlay.style.cssText = "position:fixed;pointer-events:none;z-index:2147483647;border:2px solid #e8590c;background:#e8590c18;display:none;box-sizing:border-box";
 document.documentElement.appendChild(overlay);
 const emit = (type, element) => parent.postMessage({type, token:config.token, element}, config.origin);
 const stop = () => { active=false; overlay.style.display="none"; };
 const describe = (el) => {
  const path=[];
  for(let p=el;p && p!==document.body && path.length<6;p=p.parentElement) {
   const id=p.getAttribute("data-testid") || p.id;
   path.unshift(p.tagName.toLowerCase()+(id ? "["+id+"]" : ":nth-of-type("+(Array.from(p.parentElement?.children || []).filter(s=>s.tagName===p.tagName).indexOf(p)+1)+")"));
  }
  return {tag:el.tagName.toLowerCase(),text:(el.textContent || "").replace(/\s+/g," ").trim().slice(0,300),label:el.getAttribute("aria-label") || el.getAttribute("title") || el.getAttribute("placeholder") || "",testid:el.closest("[data-testid]")?.getAttribute("data-testid") || "",role:el.getAttribute("role") || "",page:document.title,url:location.href,path:path.join(" > ")};
 };
 document.addEventListener("pointermove",e=>{
  if(!active || !(e.target instanceof Element)) return;
  const r=e.target.getBoundingClientRect();
  Object.assign(overlay.style,{display:"block",left:r.left+"px",top:r.top+"px",width:r.width+"px",height:r.height+"px"});
 },true);
 document.addEventListener("click",e=>{
  if(!active || !(e.target instanceof Element)) return;
  e.preventDefault(); e.stopImmediatePropagation();
  emit("archetipo-picked",describe(e.target)); stop();
 },true);
 document.addEventListener("keydown",e=>{if(active && e.key==="Escape"){e.preventDefault();stop();emit("archetipo-picker-cancelled");}},true);
 addEventListener("message",e=>{
  if(e.source!==parent || e.origin!==config.origin || e.data?.token!==config.token) return;
  if(e.data.type==="archetipo-picker-hello") emit("archetipo-picker-ready");
  if(e.data.type==="archetipo-picker-toggle"){active=Boolean(e.data.active);if(!active)stop();}
 });
 emit("archetipo-picker-ready");
}
