import {test} from 'node:test';
import assert from 'node:assert/strict';
import {rollbackTarget} from './rollback-target.mjs';
const first='11111111-1111-1111-1111-111111111111';
const current='22222222-2222-2222-2222-222222222222';
const deployment=(id,on,percentage=100)=>({created_on:on,versions:[{version_id:id,percentage}]});
test('pins newest pre-release version regardless of list order',()=>{
 const rows=[deployment(first,'2026-09-02T12:00:00Z'),deployment(current,'2026-09-03T12:00:00Z')];
 assert.equal(rollbackTarget(rows),current);assert.equal(rollbackTarget([...rows].reverse()),current);
 assert.equal(rows[0].versions[0].version_id,first);
});
test('first deployment has no rollback target',()=>assert.equal(rollbackTarget([]),''));
test('malformed or split deployment cannot supply a command argument',()=>{
 for(const rows of [{},[deployment('$(bad)','2026-09-03T12:00:00Z')],[deployment(current,'invalid')],[deployment(current,'2026-09-03T12:00:00Z',50)]]) {
  assert.throws(()=>rollbackTarget(rows));
 }
});
