# Home page variants (prototype)

Throwaway prototype answering: **what should the Cave LMS site look like?**
Three radically different directions for the home page, switchable with a floating
bottom bar (`←`/`→` arrow keys work too).

    ./run.sh          # then open http://localhost:4173/?variant=A

- `A · Catalogue` — institutional: dense course catalogue, paper-and-green palette, serif headings
- `B · Cinematic` — media-first: dark, big type, video plate, horizontal rails
- `C · Reader` — typographic: narrow column, numbered list, almost no chrome

Placeholder content only; nothing is wired to a backend. Capture: winner (or the
mix of bits) folds into the real Go templates; the full set stays on the
`prototype/mockup` branch as the primary source.
