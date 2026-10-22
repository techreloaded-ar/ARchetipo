// Pure descriptions used by the preview panel and its credential-free tests.
(function(root) {
 "use strict";
 function commentMessage(comments) {
  return "Richieste di modifica dalla preview. I riferimenti descrivono la pagina osservata; verifica i sorgenti prima di intervenire.\n\n" + comments.map((c,i) => {
   const e=c.element || {};
   return `${i+1}. Pagina: ${e.page || ""}\nURL: ${e.url || ""}\nElemento: ${e.tag || ""}; testo: ${JSON.stringify(e.text || "")}; etichetta: ${JSON.stringify(e.label || "")}; data-testid: ${JSON.stringify(e.testid || "")}; ruolo: ${e.role || ""}\nPercorso: ${e.path || ""}\nRichiesta: ${c.note.trim()}`;
  }).join("\n\n");
 }
 function setupMessage() {
  return 'Configura o ripara la preview browser del progetto per questa conversazione e per gli incrementi successivi. Esamina il progetto e la ricetta esistente; individua comando di avvio e directory locali senza indovinare il framework. Leggi `archetipo execution environment show`; se manca la ricetta, definisci un setup idempotente con dipendenze locali, risorse e dati sintetici appartenenti alla directory di lavoro, readiness probe, cleanup e controlli usando gli script del progetto. Persisti con `archetipo execution environment configure --file -` il JSON version 1 {"version":1,"inputs":["file-di-dipendenze-e-lockfile"],"outputs":["output-locale"],"prepare":[{"name":"setup","argv":["comando","argomento"],"timeout_seconds":300}],"probe":[{"name":"ready","argv":["comando-di-verifica"],"timeout_seconds":30}],"checks":{}} adattato al progetto; includi gli input effettivi, senza segreti. Persisti la preview dalla tua directory di lavoro con `archetipo execution preview configure --file -`, passando via stdin JSON {"version":1,"directory":".","argv":["comando","argomento-porta","{port}"],"url":"http://127.0.0.1:{port}/percorso","prepare_environment":true}. Usa il segnaposto {port} sia nella porta URL sia negli argomenti del comando, con binding locale e fallimento se la porta è occupata; ARchetipo lo sostituisce ad ogni avvio. La directory deve essere relativa alla directory di lavoro della conversazione. Mantieni le due ricette e gli eventuali helper versionati con il progetto, così gli incrementi successivi le ereditano tramite main; non copiare checkpoint, credenziali o dipendenze da altri checkout. Esegui `archetipo execution preview prepare` e verifica preparation.passed e preparation.stable, leggendo il report originale anche su riuso. Non avviare il server: lo avvierò con il pulsante Avvia preview, che preparerà e verificherà automaticamente l’ambiente. Se non è possibile visualizzare il risultato o mancano servizi/credenziali, spiega il blocco concreto. Non modificare l’applicazione per aggiungere il selettore: lo fornisce ARchetipo.';
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
 const api={commentMessage,setupMessage,cleanElement,panelLimits,panelHeight,panelState};
 if(typeof module!=="undefined" && module.exports) module.exports=api;
 else root.Preview=api;
})(typeof window!=="undefined"?window:globalThis);
