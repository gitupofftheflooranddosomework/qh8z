# Third-party notices

QH8Z is built with and informed by open-source software. This file is intended to preserve provenance and make future commercial due diligence straightforward.

## Shlink

- Project: Shlink
- Repository: `shlinkio/shlink`
- Initial QH8Z baseline: v5.1.5
- Role: runtime redirect and visit-tracking engine
- License: MIT
- License text: [`licenses/SHLINK-MIT.txt`](licenses/SHLINK-MIT.txt)

QH8Z communicates with Shlink through its REST API and deploys the official Shlink container. Shlink remains a distinct third-party component.

## Kutt

- Project: Kutt
- Repository: `thedevs-network/kutt`
- Initial QH8Z reference baseline: v3.2.6
- Role: product/UX implementation reference and potential selective code donor
- License: MIT
- License text: [`licenses/KUTT-MIT.txt`](licenses/KUTT-MIT.txt)

At this stage QH8Z does not run the Kutt application as a second shortening engine. If substantial Kutt code is directly ported later, provenance should be recorded at the file/feature level.

## npm/container dependencies

QH8Z also depends on third-party Node packages and container images declared in `services/app/package.json` and `docker-compose.yml`. Their licenses remain their own. Dependency license auditing should be part of release due diligence.
