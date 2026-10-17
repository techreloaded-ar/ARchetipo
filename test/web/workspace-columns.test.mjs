import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {preferences,inspector}=require('../../cli/internal/web/assets/workspace-columns.js');

test('saved column widths stay within usable bounds',()=>{
 assert.deepEqual(preferences(),{rail:264,ratio:.48,collapsed:false});
 assert.deepEqual(preferences({rail:800,ratio:3,collapsed:true}),{rail:420,ratio:.68,collapsed:true});
 assert.deepEqual(preferences({rail:-10,ratio:-2}),{rail:190,ratio:.28,collapsed:false});
});
test('the inspector leaves room for both the chat and its details',()=>{
 assert.equal(inspector(1000,.48),480);
 assert.equal(inspector(1000,.1),300);
 assert.equal(inspector(1000,.99),700);
 assert.equal(inspector(650,.68),350);
});
