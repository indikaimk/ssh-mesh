import { defineConfig } from 'vitepress';

export default defineConfig({
  title: 'SSH Mesh',
  description: 'Zero-Inbound Reverse SSH Control Plane & Web Terminal for Private Cloud VMs',
  base: '/ssh-mesh/',
  appearance: 'dark',
  cleanUrls: true,
  themeConfig: {
    nav: [
      { text: 'Guide', link: '/getting-started' },
      { text: 'Architecture', link: '/architecture' },
      { text: 'RBAC & Tags', link: '/rbac-and-tags' },
      { text: 'Auditing & Replay', link: '/auditing-and-replay' },
      { text: 'API Reference', link: '/api-reference' },
    ],
    sidebar: [
      {
        text: 'Introduction',
        items: [
          { text: 'Getting Started', link: '/getting-started' },
          { text: 'Architecture & Tunneling', link: '/architecture' },
        ],
      },
      {
        text: 'Security & Governance',
        items: [
          { text: 'RBAC & Tag Access Control', link: '/rbac-and-tags' },
          { text: 'Auditing & Session Replay', link: '/auditing-and-replay' },
        ],
      },
      {
        text: 'Operations',
        items: [
          { text: 'Production Deployment', link: '/deployment' },
          { text: 'REST & WebSocket API', link: '/api-reference' },
        ],
      },
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/indikaimk/ssh-mesh' },
    ],
    search: {
      provider: 'local',
    },
    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © 2026 SSH Mesh Contributors',
    },
  },
});

