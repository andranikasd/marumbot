// One production payload for both the edge and the embedded Go server.
import { build } from 'esbuild';
import { cp, mkdir, readFile, writeFile, rm } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { resolve, relative } from 'node:path';

const source = fileURLToPath(new URL('../../internal/adapter/in/miniapp/web/', import.meta.url));
const output = fileURLToPath(new URL('./dist/assets/', import.meta.url));
await rm(output, {recursive:true, force:true});
await mkdir(output, { recursive: true });
await cp(source, output, { recursive: true });
const result = await build({ metafile: true, entryPoints: [resolve(source, 'js/main.js')], outdir: resolve(output, 'js'), splitting: true, chunkNames: 'chunks/[name]-[hash]', bundle: true, minify: true, format: 'esm', target: 'safari15', legalComments: 'none' });
await build({ entryPoints: [resolve(source, 'styles.css')], outfile: resolve(output, 'styles.css'), minify: true });
const shell = await readFile(resolve(source, 'index.html'), 'utf8');
// Source modules remain available for diagnostics, but the browser fetches only
// the bundle. Keeping source preloads would defeat bundling's network benefit.
const initial = new Set();
function visit(path) {
  if(initial.has(path)) return;
  initial.add(path);
  for(const dependency of result.metafile.outputs[path].imports) {
    if(dependency.kind === 'import-statement' && !dependency.external) visit(dependency.path);
  }
}
visit(Object.keys(result.metafile.outputs).find(path => resolve(path) === resolve(output, 'js/main.js')));
const preloads = [...initial].map(path => `<link rel="modulepreload" href="${relative(output, resolve(path)).split('\\').join('/')}">`).join('\n');
await writeFile(resolve(output, 'index.html'), shell.replace(/^<link rel="modulepreload"[^>]*>\n/gm, '').replace('</head>', preloads+'\n</head>'));
await writeFile(resolve(output, '../build-meta.json'), JSON.stringify(result.metafile));
console.log(`Initial JavaScript: ${[...initial].reduce((sum,path)=>sum+result.metafile.outputs[path].bytes,0)} bytes across ${initial.size} minified files; secondary tools load on demand.`);
