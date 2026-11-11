import assert from "node:assert/strict";
import {readFile, readdir} from "node:fs/promises";
import {test} from "node:test";
const root = new URL("../../skills/", import.meta.url);
test("Retired delivery methods are absent from the distribution", async () => {
 const names = await readdir(root);
 for (const retired of ["autopilot","design"]) {
  const directory="archetipo-"+retired;
  if (!names.includes(directory)) continue;
  const files=await readdir(new URL(directory+"/",root));
  assert.ok(!files.includes("SKILL.md"),directory+" is still installed");
 }
});
test("Product work records enterprise context without an internal delivery pipeline", async () => {
 const entry = await readFile(new URL("archetipo-increment/SKILL.md",root),"utf8");
 for (const method of ["legacy.md","/archetipo-autopilot","/archetipo-implement","/archetipo-review"]) assert.ok(!entry.includes(method));
 for (const operation of ["increment learn","increment conclude","increment link","story_codes","specification"]) assert.ok(entry.includes(operation));
});
test("Classic story implementation stays outside the increment workflow", async () => {
 const entry = await readFile(new URL("archetipo-implement/SKILL.md",root),"utf8");
 for (const operation of ["archetipo increment","increment propose","increment approve","increment ready","gh pr"]) assert.ok(!entry.includes(operation),operation);
 for (const operation of ["spec start","task done","spec review","data.workdir",'"status":"REVIEW"']) assert.ok(entry.includes(operation),operation);
});
test("Classic story review leaves the verdict to the person, outside the increment workflow", async () => {
 const entry = await readFile(new URL("archetipo-review/SKILL.md",root),"utf8");
 for (const operation of ["archetipo increment","increment ready","gh pr"]) assert.ok(!entry.includes(operation),operation);
 for (const operation of ["spec review-dossier","spec request-changes","spec integrate","spec move","prepared dossier mode",'"status":"REVIEW"']) assert.ok(entry.includes(operation),operation);
});
