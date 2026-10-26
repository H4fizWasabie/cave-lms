---
status: accepted, supersedes ADR-0004
---

# Deployments live under a path on cavelms.com

Amended by ADR 0017: a Deployment's page is the Learner's own HTML, under a policy that allows a fixed list of CDNs and sends no data out.

When Cave LMS moves to `cavelms.com`, Deployments move with it to `cavelms.com/<prefix>/<name>`, where the Learner picks the name and the client picks the prefix word. They keep the `Content-Security-Policy: sandbox allow-scripts` header from ADR 0004, so a Dashboard's scripts still cannot touch Cave's cookies. The client wants one address for everything, and a path needs no extra domain, DNS record or certificate. The accepted cost is reputation: Learners now upload their own data and pick readable names, so a Dashboard flagged as phishing can get all of `cavelms.com` flagged, login pages included; Takedown is the answer. A fixed prefix keeps Learner names apart from LMS routes, so a new LMS page can never collide with a name already taken. Rejected: (1) a separate Deployment domain, the stronger wall, which the client declined; (2) `<name>.cavelms.com` subdomains, which need a wildcard certificate and can set cookies for the parent domain; (3) names at the root, which collide with LMS routes. Revisit after the first phishing flag: moving the prefix to another domain is a redirect, not a rewrite. Old `lms.wasabietech.com/d/<slug>` links follow ADR 0002.
