// Copies an explicit allowlist of docs from their source repos into the site.
// Only listed files are published; anything else in docs/ (local notes, drafts) never is.
// Relative links that point outside the copied set are rewritten to GitHub URLs at the
// pinned commit, so code references keep working on the site.
import { execSync } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join, posix, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const site = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const repoRoot = resolve(site, '../..')

const rev = (dir) => execSync('git rev-parse HEAD', { cwd: dir }).toString().trim()

const sources = [
  {
    out: 'middleware',
    root: repoRoot,
    from: 'docs',
    github: `https://github.com/ChainSafe/canton-middleware/blob/${process.env.GITHUB_SHA || rev(repoRoot)}`,
    files: [
      'ARCHITECTURE.md',
      'SECURITY_AND_PRIVACY_MODEL.md',
      'SIGNER_ARCHITECTURE.md',
      'CIP-0086-OVERVIEW.md',
      'API_DOCUMENTATION.md',
      'WAYFINDER_DEPLOYMENT_REQUIREMENTS.md',
      'E2E_TEST_ARCHITECTURE.md',
      'DEVNET_INTEROP_TESTING.md',
      'LOCAL_INTEROP_TESTING.md',
      'DAPP_SNAP_TESTING.md',
      'openapi/README.md',
    ],
  },
  {
    out: 'contracts',
    root: resolve(repoRoot, 'contracts/canton-erc20'),
    from: 'docs',
    github: `https://github.com/ChainSafe/canton-erc20/blob/${rev(resolve(repoRoot, 'contracts/canton-erc20'))}`,
    files: [
      'README.md',
      'ARCHITECTURE_DIAGRAMS.md',
      'ISSUER_CENTRIC_MODEL.md',
      'middleware-bridge-architecture.md',
      'startup-flow.md',
      'dev-setup.md',
      'E2E-TESTNET-SETUP.md',
      'architecture/diagram.md',
    ],
  },
  {
    out: 'snap',
    root: process.env.SNAP_DIR ? resolve(process.env.SNAP_DIR) : null,
    from: '',
    github: `https://github.com/ChainSafe/canton-snap/blob/${process.env.SNAP_REF || 'main'}`,
    files: ['README.md', 'docs/testing-with-middleware.md'],
  },
]

const linkRe = /(\]\()([^)\s]+)((?:\s+"[^"]*")?\))/g

for (const src of sources) {
  const outDir = join(site, src.out)
  rmSync(outDir, { recursive: true, force: true })
  if (!src.root || !existsSync(src.root)) {
    console.warn(`sync: skipping ${src.out} (source not available)`)
    continue
  }
  const published = new Set(src.files.map((f) => posix.join(src.from, f)))
  for (const file of src.files) {
    const srcPath = posix.join(src.from, file)
    const text = readFileSync(join(src.root, srcPath), 'utf8').replace(linkRe, (m, open, target, close) => {
      if (/^([a-z]+:|#|\/)/i.test(target)) return m
      const [path, anchor = ''] = target.split('#')
      const resolved = posix.normalize(posix.join(posix.dirname(srcPath), path))
      if (published.has(resolved)) return m
      return `${open}${src.github}/${resolved}${anchor ? `#${anchor}` : ''}${close}`
    })
    const dest = join(outDir, file)
    mkdirSync(dirname(dest), { recursive: true })
    writeFileSync(dest, text)
  }
  console.log(`sync: ${src.out} <- ${relative(repoRoot, src.root) || '.'} (${src.files.length} files)`)
}
