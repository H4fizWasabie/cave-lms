# Home page variants (prototype)

Throwaway prototype answering: **what should the Cave LMS site look like?**
Two shortlisted directions for the home page, switchable with a floating bottom
bar (`←`/`→` arrow keys work too). A third direction (`C · Reader`) was built
and rejected during review; it lives on in git history only.

    ./run.sh          # then open http://localhost:4173/?variant=A

- `A · Catalogue` — institutional: course catalogue, paper-and-green palette,
  Source Serif 4 headings over Public Sans body
- `B · Cinematic` — media-first: dark, big Archivo display type, video plate,
  horizontal rails

Placeholder content only; nothing is wired to a backend.

## Assets

- `fonts/` — self-hosted WOFF2 (Fontsource builds of Source Serif 4, Public Sans,
  Archivo). No requests to font CDNs.
- Photos are **[picsum.photos](https://picsum.photos) placeholders** (`seed/…`
  URLs, so the same image comes back every load). They need network access.
  Swap for the client's own promotional material before any outside review.

## Capture

- Winner is chosen by reviewing the running prototype, not this code.
- Then it gets folded into the real Go templates (`internal/app`); this
  directory is never shipped.
- The full set stays on the `prototype/mockup` branch as the primary source.
