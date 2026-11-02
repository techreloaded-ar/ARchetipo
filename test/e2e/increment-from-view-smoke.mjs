#!/usr/bin/env node
// Same real product flow, entered from the enterprise backlog. This variant
// also exercises recovery, publication, developer changes and bidirectional trace.
process.argv.push("--enterprise");
await import("./chat-first-view-smoke.mjs");
