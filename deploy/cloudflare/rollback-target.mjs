import {readFileSync} from 'node:fs';
import {pathToFileURL} from 'node:url';

// Wrangler lists deployments oldest first. Pin the currently active version,
// before code deployment and secret synchronization create further revisions.
export function rollbackTarget(deployments) {
  if (!Array.isArray(deployments)) throw new Error('Invalid deployment list');
  if (!deployments.length) return '';
  for (const d of deployments) {
    if (!Number.isFinite(Date.parse(d.created_on))) throw new Error('Invalid deployment date');
  }
  const latest = [...deployments].sort((a,b)=>Date.parse(b.created_on)-Date.parse(a.created_on))[0];
  if (!Array.isArray(latest.versions) || latest.versions.length !== 1 || latest.versions[0].percentage !== 100) {
    throw new Error('Cannot select a single rollback target from split traffic');
  }
  const id = latest.versions[0].version_id;
  if (typeof id !== 'string' || !/^[a-f0-9]{8}(?:-[a-f0-9]{4}){3}-[a-f0-9]{12}$/i.test(id)) {
    throw new Error('Invalid Worker version identifier');
  }
  return id;
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const id = rollbackTarget(JSON.parse(readFileSync(0,'utf8')));
  process.stdout.write(`version_id=${id}\n`);
}
