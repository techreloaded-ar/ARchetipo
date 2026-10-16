import assert from "node:assert/strict";
import { readFile, readdir } from "node:fs/promises";
import { test } from "node:test";

const root=new URL("../../skills/",import.meta.url);
async function instructions(name) {
 const url=new URL(name+"/",root);
 const paths=await readdir(url,{recursive:true});
 return (await Promise.all(paths.filter(p=>p.endsWith(".md")).map(p=>readFile(new URL(p,url),"utf8")))).join("\n");
}

test("base workflow instructions remain independent of composing skills", async()=>{
 for(const name of ["inception","design","spec","plan","implement","review"]) {
  const body=await instructions("archetipo-"+name);
  for(const dependency of ["archetipo-autopilot","archetipo-increment","data.increment"])
   assert.ok(!body.includes(dependency),`${name} depends on outer controller ${dependency}`);
 }
});

test("Autopilot composes technical phases without an increment dependency",async()=>{
 const body=await instructions("archetipo-autopilot");
 for(const dependency of ["archetipo-increment","data.increment","increment ready","increment publish"])
  assert.ok(!body.includes(dependency),`Autopilot depends on ${dependency}`);
});
