// viewer-console-probe.mjs
//
// Sonda funzionante usata per diagnosticare la regressione del rail delle
// conversazioni (28/08/2026). Non è un test: è il punto di partenza da cui
// ricavarne uno.
//
// Carica la pagina del viewer in Chrome headless e riporta ogni eccezione non
// catturata emessa durante il caricamento. È esattamente il controllo che
// avrebbe colto il bug: un TypeError a top level nell'IIFE di app.js spegneva
// in silenzio tutti i bind che seguivano, e nessun test se ne accorgeva.
//
// Nessuna dipendenza nuova: Chrome è già installato sulla macchina e Node 22+
// espone WebSocket come globale, quindi CDP si parla senza `ws`.
//
// Uso:
//   1. avvia Chrome:
//      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
//        --headless=new --disable-gpu --remote-debugging-port=9222 \
//        --user-data-dir=/tmp/chrome-probe --no-first-run about:blank &
//   2. avvia il viewer:  archetipo view --port 8099 --no-open
//   3. node docs/plans/viewer-console-probe.mjs http://localhost:8099/

const cdpBase = "http://127.0.0.1:9222";
const pageUrl = process.argv[2] || "http://localhost:8080/";

let nextId = 1;
function rpc(ws, method, params) {
	const id = nextId++;
	return new Promise((resolve, reject) => {
		const onMessage = (event) => {
			const msg = JSON.parse(event.data);
			if (msg.id !== id) return;
			ws.removeEventListener("message", onMessage);
			if (msg.error) reject(new Error(JSON.stringify(msg.error)));
			else resolve(msg.result);
		};
		ws.addEventListener("message", onMessage);
		ws.send(JSON.stringify({ id, method, params: params || {} }));
	});
}

const target = await (
	await fetch(`${cdpBase}/json/new?${encodeURIComponent("about:blank")}`, {
		method: "PUT",
	})
).json();
const ws = new WebSocket(target.webSocketDebuggerUrl);
await new Promise((r) => ws.addEventListener("open", r, { once: true }));

const exceptions = [];
ws.addEventListener("message", (event) => {
	const msg = JSON.parse(event.data);
	if (msg.method !== "Runtime.exceptionThrown") return;
	const details = msg.params.exceptionDetails;
	exceptions.push({
		text: details.text,
		description: (details.exception && details.exception.description) || "",
		url: details.url,
		line: details.lineNumber,
	});
});

await rpc(ws, "Runtime.enable");
await rpc(ws, "Page.enable");
await rpc(ws, "Page.navigate", { url: pageUrl });
await new Promise((r) => setTimeout(r, 5000));

async function evaluate(expression) {
	const res = await rpc(ws, "Runtime.evaluate", {
		expression,
		returnByValue: true,
		awaitPromise: true,
	});
	if (res.exceptionDetails) {
		return {
			__error:
				(res.exceptionDetails.exception &&
					res.exceptionDetails.exception.description) ||
				res.exceptionDetails.text,
		};
	}
	return res.result.value;
}

console.log("=== eccezioni al caricamento ===");
console.log(JSON.stringify(exceptions, null, 2));

// Il rail è disegnato ma i suoi comandi rispondono? Un pulsante visibile e
// cliccabile che non fa niente è il sintomo esatto del bind mai registrato.
console.log("\n=== il click su un thread apre la conversazione? ===");
exceptions.length = 0;
console.log(
	await evaluate(`(() => {
	const t = document.querySelector("#workspace-conversations [data-conversation-id]");
	if (!t) return "nessun thread nel rail";
	t.click();
	return "cliccato " + t.getAttribute("data-conversation-id");
})()`),
);
await new Promise((r) => setTimeout(r, 3000));
console.log(
	JSON.stringify(
		await evaluate(`({
	corrente: document.querySelector(".thread.is-current")?.getAttribute("data-conversation-id") || null,
	pannello: document.getElementById("workspace-conversation")?.innerHTML.length,
})`),
		null,
		2,
	),
);
console.log("eccezioni dopo il click:", JSON.stringify(exceptions, null, 2));

await fetch(`${cdpBase}/json/close/${target.id}`);
ws.close();
process.exit(0);
