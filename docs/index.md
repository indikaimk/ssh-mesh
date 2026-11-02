---
layout: home

hero:
  name: "SSH Mesh"
  text: "Zero-Inbound Reverse SSH for Private VMs"
  tagline: "Access cloud VMs and VPS instances in private subnets with zero public IPs, zero port forwarding, and zero inbound firewall rules."
  actions:
    - theme: brand
      text: Get Started
      link: /getting-started
    - theme: alt
      text: Architecture
      link: /architecture
    - theme: alt
      text: GitHub
      link: https://github.com/indikaimk/ssh-mesh

features:
  - title: Outbound-Only Yamux Tunnel
    details: Agents dial outbound over TLS WebSocket and upgrade to a multiplexed Yamux session. No OpenSSH daemon or open port 22 required on target VMs.
  - title: Multi-User RBAC & Tag Access
    details: Assign Admin, Operator, or Viewer roles and restrict users to specific VMs or tags (e.g. env:staging, team:backend) via a visual node picker.
  - title: 3-Layer Security Auditing & Replay
    details: Opt-in control-plane audit trail, shell command history capture, and full asciicast v2 terminal recording with built-in xterm.js playback.
  - title: One-Line Auto-Detecting Installer
    details: Enroll Linux and macOS nodes (x86_64 & ARM64) with a single curl command that downloads static binaries and configures systemd automatically.
---

