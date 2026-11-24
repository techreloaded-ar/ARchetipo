import {test} from "node:test";
import assert from "node:assert/strict";
import {createRequire} from "node:module";
const require=createRequire(import.meta.url);
const {renderWorks,renderStories}=require("../../cli/internal/web/assets/story-trace.js");
test("Enterprise trace connects original stories, work and developer PR with escaped text",()=>{
 const html=renderWorks([{title:'<work>',conversation_id:'c" onclick="bad',status_label:'Future server label',pr_url:'https://example.test/pull/1'}]);
 assert.match(html,/Future server label/);
 assert.match(html,/data-linked-conversation="c&quot;/);
 assert.match(html,/&lt;work&gt;/);
 assert.match(html,/rel="noopener noreferrer"/);
 assert.doesNotMatch(renderWorks([{pr_url:'javascript:bad()'}]),/href=/);
 assert.match(renderStories([{code:'US-001',title:'Selection',status_label:'Waiting for business'}]),/data-linked-story="US-001"/);
 assert.doesNotMatch(renderStories([]),/story-links/);
});
test("A work says what it means for the story only in the server's own words",()=>{
 const html=renderWorks([{title:'Increment',conversation_id:'conv-1',status_label:'In revisione ai developer',note:'Server <note> about the closure'}]);
 assert.match(html,/Server &lt;note&gt; about the closure/);
 assert.doesNotMatch(renderWorks([{title:'Increment',status_label:'Lavoro in corso'}]),/docs-note/);
});
