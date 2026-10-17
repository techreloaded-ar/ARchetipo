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
  return 'Prepara una preview browser di quanto stiamo realizzando in questa conversazione. Esamina il progetto e individua comando di avvio, directory e URL locali; non indovinare il framework. Persisti la ricetta dalla tua directory di lavoro con `archetipo execution preview configure --file -`, passando via stdin JSON {"version":1,"directory":".","argv":["comando","argomento"],"url":"http://127.0.0.1:PORTA/percorso"}. La directory deve essere relativa alla directory di lavoro della conversazione. Se il server è già gestito altrove puoi omettere argv. Non avviare il server: lo avvierò con il pulsante Avvia preview. Usa una porta dedicata alla lavorazione. Prepara le dipendenze necessarie con il contratto del progetto; se non è possibile visualizzare il risultato o mancano servizi/credenziali, spiegalo. Non modificare l’applicazione per aggiungere il selettore: lo fornisce ARchetipo.';
 }
 function cleanElement(value) {
  if (!value || typeof value!=="object" || typeof value.tag!=="string" || typeof value.url!=="string") return null;
  const result={};
  for(const key of ["tag","text","label","testid","role","page","url","path","selector"]) result[key]=typeof value[key]==="string" ? value[key].slice(0,(key==="path" || key==="selector")?2000:1000) : "";
  return result;
 }
 const api={commentMessage,setupMessage,cleanElement};
 if(typeof module!=="undefined" && module.exports) module.exports=api;
 else root.Preview=api;
})(typeof window!=="undefined"?window:globalThis);
