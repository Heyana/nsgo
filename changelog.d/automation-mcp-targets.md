### Fixed

- Honor an explicit loopback `-host` in `nsgo dev -target web -automation` and pass the automation extra into `nsgo dev -target android -android-host -automation` so its Activity creates `TestAgent` (#1286).
- Vet `windows` and `web` automation builds in `nsgo check -target` and run end-to-end `nsgo mcp` checks for web and Linux in CI (#1286).
