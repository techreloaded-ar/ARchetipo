import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const fields = require("../../cli/internal/web/assets/provider-fields.js");
const conversation = require("../../cli/internal/web/assets/conversation.js");

test("permission profiles are provider declarations independent of models", () => {
  const view = {available:false,permission_profile:"partial",permission_profiles:[
    {id:"partial",label:"Modalità futura",description:"Confine del provider <speciale>"},
    {id:"wide",label:"Ampia",description:"Nessun limite alla cartella"}
  ]};
  const html = fields.renderConversationModelChoice(view,null,"permissions");
  assert.match(html,/data-conversation-permission-choice="partial"/);
  assert.match(html,/aria-expanded="true"/);
  assert.match(html,/Confine del provider &lt;speciale&gt;/);
  assert.doesNotMatch(html,/data-conversation-permission-choice="workspace"/);
  assert.doesNotMatch(fields.renderConversationModelChoice({available:false},null,"permissions"),/data-conversation-permission-choice/);
  const selected = fields.renderConversationModelChoice(view,{permission_profile:"wide"},"");
  assert.match(selected,/aria-checked="true" data-conversation-permission-choice="wide"/);
});

test("workspace defaults offer only declared choices and their limits", () => {
  const html = fields.renderProviderFields({config_fields:[{name:"custom_policy",label:"Permessi",choices:[{id:"specific",label:"Specifico",description:"Perimetro dichiarato"}]}]}, {custom_policy:"specific"});
  assert.match(html,/name="provider_custom_policy"/);
  assert.match(html,/value="specific" selected/);
  assert.match(html,/Perimetro dichiarato/);
  assert.doesNotMatch(html,/full_access/);
});

test("extended approval explanations are visible and escaped", () => {
  const html = conversation.renderConversation({available:true,conversation:{id:"c",active:true},events:[],approvals:[{id:"a",options:[{id:"session",label:"Ricorda",description:"Vale <solo> per la sessione",scope:"session",kind:"allow"}]}]},{});
  assert.match(html,/Vale &lt;solo&gt; per la sessione/);
  assert.match(html,/data-run-option-id="session"/);
  assert.doesNotMatch(html,/data-run-option-id="allow_rule"/);
});
