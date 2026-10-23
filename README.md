# Cave LMS

## Run

Set `DATABASE_URL`, `PUBLIC_BASE_URL`, `EMAIL_FROM`, and `STORAGE_DIR`, then run:

```sh
go run ./cmd/cave-lms
```

`LISTEN_ADDR` is optional and defaults to `:8080`. The app applies embedded Postgres migrations before it starts serving.

`STORAGE_DIR` is a directory on the VPS disk that holds every uploaded Gallery file. It is created if missing, must not be under a public web root, and must be included in the nightly backups together with the database. Uploads are capped at about 2 GB each.

To run it on the VPS, see `docs/deploy.md`.

`OWNER_EMAIL` is optional. It names the Owner, the one Admin who sees the System log at `/admin/log`; without it nobody does, though errors are still logged.

`CAVETEXT_EXTENSION_IDS` is optional: the comma-separated Chrome extension IDs that may sign in to CaveText through `/cavetext/connect`. Unset, CaveText sign-in is refused. The unpacked development build's ID is `omcdclpagggfcjlhpkohpcgdmfbelohd`; add the Chrome Web Store ID once it is published.

`OPENROUTER_API_KEY` and `OPENROUTER_MODEL` are optional. With both, the AI writes Exam questions through OpenRouter; `OPENROUTER_WEB_SEARCH` (`true` by default) turns its web search on or off. There is no built-in model. **Set a monthly credit limit on the OpenRouter key**: the app has no spending cap of its own. See `docs/deploy.md` (AI writer).

`RESEND_API_KEY` is optional. With it, verification and reset emails are sent through Resend from `EMAIL_FROM`; without it, sending fails with an explicit error.

## Create the Admin

Registration can never make an Admin. Run this once per deployment, with the same environment as above:

```sh
CAVE_ADMIN_PASSWORD='a long password' go run ./cmd/cave-lms create-admin --name "Abah" --email abah@example.com
# or, keeping the password out of the environment:
printf '%s\n' 'a long password' | go run ./cmd/cave-lms create-admin --name "Abah" --email abah@example.com
```

The password comes from `CAVE_ADMIN_PASSWORD` if set, otherwise from the first line of stdin. It is never a flag. The account is created verified. If a Learner already uses that email, it is promoted and its password is replaced. The Admin logs in on the normal login page and opens `/admin`.

## Tests

```sh
./test.sh                        # all tests
./test.sh -v ./internal/app      # arguments go to go test
```

`test.sh` starts a throwaway Postgres on a Unix socket in a temp directory, runs `go test` against it and deletes it afterwards. It needs the Postgres server binaries (`initdb`, `pg_ctl`) installed, but no running server, role or `sudo`.

CI (`.github/workflows/ci.yml`) runs `go vet ./...` and `./test.sh` on every pull request and push to main; it does not deploy.

To use your own database instead, set `TEST_DATABASE_URL` to a dedicated test database and run `go test ./...`. The HTTP integration test creates a unique schema in it and drops it when the test finishes, so the user needs permission to create and drop schemas. Without `TEST_DATABASE_URL`, that test is skipped.
