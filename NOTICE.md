# Notice

Signet — a self-hosted read-it-later reader with native RSS/Atom support.

Copyright (C) 2026 Florian Thibaud

This program is free software: you can redistribute it and/or modify it under the
terms of the GNU Affero General Public License as published by the Free Software
Foundation, either version 3 of the License, or (at your option) any later
version.

This program is distributed in the hope that it will be useful, but **WITHOUT ANY
WARRANTY**; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
PARTICULAR PURPOSE. See the GNU Affero General Public License for more details. A
full copy is available in the [LICENSE](LICENSE) file and at
<https://www.gnu.org/licenses/agpl-3.0.html>.

## Trademarks

The **Signet** name and its logo are **not** covered by the AGPL-3.0 license. The
license grants you rights over the source code, not over the project's identity.
A publicly redistributed fork, or a hosted instance offered as a service, must be
published under a distinct name to avoid any confusion about its origin.

## Inspiration

Signet is inspired by [Omnivore](https://github.com/omnivore-app/omnivore)
(AGPL-3.0), discontinued in late 2024 after the team was acquired by ElevenLabs.

**No Omnivore code was reused.** The architecture, the database schema and the
entire implementation are original: Signet is a single Go binary serving an
embedded React SPA, where Omnivore was built on a set of TypeScript services. The
kinship is limited to the product idea and some UX choices.

## Third-party dependencies

Signet links against third-party libraries, all distributed under permissive
licenses (MIT, BSD or Apache-2.0). Each remains subject to its own license; the
full list and exact versions are in [go.mod](go.mod) and
[frontend/package.json](frontend/package.json).
