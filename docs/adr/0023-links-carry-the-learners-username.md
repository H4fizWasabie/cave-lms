# Links carry the Learner's Username, not their name

Amends ADR 0007 and ADR 0017. ADR 0024 moved the Deployment address to `pages.cavelms.com/<username>/<name>/`; the rest below holds there. The client wants a Learner's folder and Deployments to show whose they are in the address: `cavelms.com/my-folder/<username>` and `cavelms.com/d/<username>/<name>/`. A Learner's name is their real name, printed on their Certificates, so putting it in a public link would leak it to everyone the link is shared with. Each Learner instead has a Username, picked when registering and changeable in My profile; existing accounts were given a random one, which they can change.

The Username is for show. Cave still finds a Deployment by its name alone, which stays unique across Cave, and the folder always shows the logged-in Learner's own. So a link with an old or wrong Username, and every link shared before this as `/d/<name>/`, redirects for good to the current one, and a Username given up and taken by another Learner never takes their Deployments with it.

Rejected: (1) a slug of the real name, which leaks it; (2) finding Deployments by Username and name together, which would break every link already shared and every link whenever a Learner changes their Username; (3) asking existing Learners to pick a Username at their next login, which blocks them for little gain.
