import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'

export default withMermaid(
  defineConfig({
    title: 'Canton EVM Middleware',
    description: 'Ethereum JSON-RPC, indexer and bridge for CIP-56 tokens on Canton',
    base: process.env.DOCS_BASE || '/canton-middleware/',
    cleanUrls: true,
    lastUpdated: true,
    // Several docs show docker-compose defaults on localhost.
    ignoreDeadLinks: 'localhostLinks',
    themeConfig: {
      search: { provider: 'local' },
      socialLinks: [{ icon: 'github', link: 'https://github.com/ChainSafe/canton-middleware' }],
      nav: [
        { text: 'Get started', link: '/get-started' },
        { text: 'Architecture', link: '/middleware/ARCHITECTURE' },
        { text: 'API', link: '/middleware/API_DOCUMENTATION' },
        { text: 'Grant verification', link: '/grant/' },
      ],
      sidebar: [
        { text: 'Get started', link: '/get-started' },
        {
          text: 'Architecture',
          items: [
            { text: 'System architecture', link: '/middleware/ARCHITECTURE' },
            { text: 'Security and privacy model', link: '/middleware/SECURITY_AND_PRIVACY_MODEL' },
            { text: 'Signer architecture', link: '/middleware/SIGNER_ARCHITECTURE' },
            { text: 'CIP-0086 overview', link: '/middleware/CIP-0086-OVERVIEW' },
          ],
        },
        {
          text: 'Daml contracts',
          items: [
            { text: 'Overview', link: '/contracts/README' },
            { text: 'Issuer-centric model', link: '/contracts/ISSUER_CENTRIC_MODEL' },
            { text: 'Middleware and bridge', link: '/contracts/middleware-bridge-architecture' },
            { text: 'Architecture diagrams', link: '/contracts/ARCHITECTURE_DIAGRAMS' },
            { text: 'Package diagram', link: '/contracts/architecture/diagram' },
            { text: 'Startup flow', link: '/contracts/startup-flow' },
            { text: 'Development setup', link: '/contracts/dev-setup' },
            { text: 'TestNet end-to-end setup', link: '/contracts/E2E-TESTNET-SETUP' },
          ],
        },
        {
          text: 'API reference',
          items: [
            { text: 'API guide', link: '/middleware/API_DOCUMENTATION' },
            { text: 'OpenAPI specs', link: '/middleware/openapi/README' },
            { text: 'API server (OpenAPI)', link: '/api/api-server.html', target: '_blank' },
            { text: 'Indexer (OpenAPI)', link: '/api/indexer.html', target: '_blank' },
            { text: 'Relayer (OpenAPI)', link: '/api/relayer.html', target: '_blank' },
          ],
        },
        {
          text: 'Operations and testing',
          items: [
            { text: 'Wayfinder deployment', link: '/middleware/WAYFINDER_DEPLOYMENT_REQUIREMENTS' },
            { text: 'End-to-end tests', link: '/middleware/E2E_TEST_ARCHITECTURE' },
            { text: 'DevNet interop testing', link: '/middleware/DEVNET_INTEROP_TESTING' },
            { text: 'Local interop testing', link: '/middleware/LOCAL_INTEROP_TESTING' },
            { text: 'dApp and Snap testing', link: '/middleware/DAPP_SNAP_TESTING' },
          ],
        },
        {
          text: 'MetaMask Snap',
          items: [
            { text: 'Overview', link: '/snap/README' },
            { text: 'Testing with the middleware', link: '/snap/docs/testing-with-middleware' },
          ],
        },
        {
          text: 'Grant verification',
          items: [
            { text: 'Overview', link: '/grant/' },
            { text: 'M1: Architecture and contracts', link: '/grant/m1' },
            { text: 'M2: Middleware service', link: '/grant/m2' },
          ],
        },
      ],
      footer: {
        message: 'Apache-2.0. Built from the canton-middleware repository.',
      },
    },
  }),
)
