import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve,dirname} from 'node:path';
import {fileURLToPath} from 'node:url';
import {gzipSync} from 'node:zlib';
const root=fileURLToPath(new URL('./dist/assets/',import.meta.url));
const shell=await readFile(resolve(root,'index.html'),'utf8');
const paths=[...shell.matchAll(/rel="modulepreload" href="([^"]+)"/g)].map(match=>match[1]);
assert.ok(paths.includes('js/main.js'));
assert.ok(paths.every(path=>!path.includes('/screens/')),'source module preloads must not defeat bundling');
let total=0,compressed=0;
for(const path of paths){const body=await readFile(resolve(root,path));total+=body.length;compressed+=gzipSync(body).length;}
assert.ok(total<230000,`initial JS exceeds budget: ${total}`);
assert.ok(compressed<65000,`compressed initial JS exceeds budget: ${compressed}`);
// Follow every static/dynamic output import, including lazy forms. A broken
// chunk URL must fail CI rather than the borrower's first tap after deployment.
const visited=new Set();
async function check(path){
 if(visited.has(path))return;visited.add(path);
 const body=await readFile(path,'utf8');
 for(const match of body.matchAll(/(?:from|import)\s*\(?["'](\.\.?\/[^"']+)["']/g))await check(resolve(dirname(path),match[1]));
}
await check(resolve(root,'js/main.js'));
assert.ok([...visited].some(path=>path.includes('/add-')),'loan form must be split into a lazy chunk');
console.log(`Production assets: ${total} initial bytes, ${compressed} gzip bytes, ${visited.size} reachable files.`);
