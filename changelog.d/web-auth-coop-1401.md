### Fixed

- Web OAuth sign-ins complete when the provider's Cross-Origin-Opener-Policy cuts the popup off from the page: the redirect page hands its callback back over a same-origin `BroadcastChannel`, and a popup that reads as closed after showing the provider waits up to 5 minutes for it instead of ending the session cancelled at once (#1401 by @houko).
