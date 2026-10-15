# Cave LMS

## Run

Set `DATABASE_URL`, `PUBLIC_BASE_URL`, and `EMAIL_FROM`, then run:

```sh
go run ./cmd/cave-lms
```

`LISTEN_ADDR` is optional and defaults to `:8080`. The app applies embedded Postgres migrations before it starts serving.

Email delivery is not configured yet. The Mailer reports an explicit error until a provider is added.

## Tests

```sh
./test.sh                        # all tests
./test.sh -v ./internal/app      # arguments go to go test
```

`test.sh` starts a throwaway Postgres on a Unix socket in a temp directory, runs `go test` against it and deletes it afterwards. It needs the Postgres server binaries (`initdb`, `pg_ctl`) installed, but no running server, role or `sudo`.

To use your own database instead, set `TEST_DATABASE_URL` to a dedicated test database and run `go test ./...`. The HTTP integration test creates a unique schema in it and drops it when the test finishes, so the user needs permission to create and drop schemas. Without `TEST_DATABASE_URL`, that test is skipped.
