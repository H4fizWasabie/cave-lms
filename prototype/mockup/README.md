# The Cave Consult learning portal (clickable mockup)

Throwaway prototype answering: **what should the Cave LMS look and feel like, end to end?**
A static click-through of the three journeys, in The Cave Consult's own brand, with
their real programmes, class photos and alumni quotes.

    ./run.sh          # then open http://localhost:4173/

Use the **Demo** bar at the bottom to jump between roles:

- **Visitor**: home (Look A or Look B), courses, course page, gallery, check a
  certificate (try `TCC-4M8P-Q2RW` valid, `TCC-9D2L-X7NB` revoked), register,
  check your email, log in (with demo shortcuts)
- **Learner**: My courses, course progress, lessons (video, text, ebook), mark
  complete, exam (real scoring, 70% pass mark, retry on fail), certificate with QR
  code and "Download PDF" (the browser's print-to-PDF, A4 landscape)
- **Admin**: dashboard, courses (open/close), course editor with Details, Lessons
  (add from Gallery or as text, reorder with up/down), Exam (settings and
  questions) and Learners (progress, best score, remove), Gallery (upload with
  preview and file checks, Visibility, Promotional), Learners, Certificates
  (search, revoke and reissue, certificate design)

Nothing is saved to a server. Demo progress lives in the browser tab
(`sessionStorage`), so a journey hangs together across pages; **Reset** or closing
the tab starts over. Only Look A is built out beyond the home page.

## Files

- `index.html` and the other `*.html` pages: one per screen
- `data.js`: all sample data in one place (courses, lessons, exam, people, certificates)
- `app.js`: shared header, footer, admin sidebar, demo tour and every scripted moment
- `styles.css`: public and Learner screens, plus home Look B; `admin.css`: admin workspace

## Swapping the brand

- Colours and fonts: the **BRAND** block at the top of `styles.css` (ink, gold, deep
  gold for text, soft gold, display and body fonts).
- Logo: replace `img/logo.svg` (for light backgrounds) and `img/logo-light.svg` (for
  dark backgrounds). Favicon: `img/favicon.png`.
- Organisation name, contact details and certificate signatory: `org` in `data.js`.

## Assets

- `img/class-*.jpg`, `img/event-*.jpg`, `img/logo*.svg`, `img/favicon.png`: The Cave
  Consult's own public material from thecaveconsult.com, used for this pitch.
- `img/hero-learner.jpg`, `img/prospectus.jpg`: free stock from
  [Unsplash](https://unsplash.com/license) (`eQryxYGXsu0`, `e_P2MEgzmzQ`).
- `img/arch.jpg`: a Blender render of the gold arch from their logo (log-in and
  register pages). Re-render with
  `blender -b --factory-startup -P img/arch.blender.py -- img/arch.jpg 256`.
- `img/icons.svg`: [Phosphor Icons](https://phosphoricons.com) (MIT), as a sprite.
- `img/qr-*.svg`: QR codes for the sample certificate IDs.
- `fonts/`: self-hosted WOFF2 (Fontsource builds of Source Serif 4, Public Sans,
  Archivo, and Allura for the signature). No requests to font CDNs.
- Sample people, numbers, certificate IDs and the signatory name are invented.

## Publish

    ./deploy.sh                            # https://cave.wasabietech.com/
    DOMAIN=demo.example.com ./deploy.sh    # move it: point the new domain's A record at the VPS first

Copies the files to `vps:/var/www/cave-mockup` and writes the one Caddy site file
`/etc/caddy/sites/cave-mockup.caddy` (domain on its first line, imported from the main
Caddyfile). It validates the Caddy config before reloading and restores the previous
site file if validation fails. Pages are served with `noindex`.

## Capture

- The winning look gets folded into the real Go templates (`internal/app`); this
  directory is never shipped.
- The full set stays on the `prototype/mockup` branch as the primary source.
