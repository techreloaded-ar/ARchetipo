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
 const api={commentMessage,cleanElement,panelLimits,panelHeight,panelState};
 if(typeof module!=="undefined" && module.exports) module.exports=api;
 else root.Preview=api;
})(typeof window!=="undefined"?window:globalThis);
