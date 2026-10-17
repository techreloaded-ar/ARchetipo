import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { test } from "node:test";
const require = createRequire(import.meta.url);
const { renderConversation } = require("../../cli/internal/web/assets/conversation.js");
const { renderWorkspaceRuns } = require("../../cli/internal/web/assets/workspace-runs.js");

test("execution progress echoes server labels and escapes diagnostics", () => {
 const html = renderConversation({available:true,conversation:{id:"conv-1",state:"ACTIVE"},events:[],workflow:{label:"Etichetta del server",detail:"<script>diagnostica</script>"}},"bozza",{});
 assert.match(html,/Etichetta del server/);
 assert.match(html,/&lt;script&gt;diagnostica&lt;\/script&gt;/);
 assert.doesNotMatch(html,/<script>/);
 assert.match(html,/bozza/);
});

test("conversation work appears in the board without a nonexistent execution stop", () => {
 const html=renderWorkspaceRuns({runs:[{id:"conv-1",conversation_id:"conv-1",scope:"workspace",action:"Attività in conversazione",status:"In corso",can_stop:false}]});
 assert.match(html,/Attività in conversazione/);
 assert.match(html,/data-run-id="conv-1"/);
 assert.doesNotMatch(html,/data-run-stop/);
 assert.doesNotMatch(html,/In questo workspace non sta girando niente/);
});
