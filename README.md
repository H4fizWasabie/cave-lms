# Cave LMS

## Run

Set `DATABASE_URL`, `PUBLIC_BASE_URL`, and `EMAIL_FROM`, then run:

```sh
go run ./cmd/cave-lms
```

`LISTEN_ADDR` is optional and defaults to `:8080`. The app applies embedded Postgres migrations before it starts serving.

Email delivery is not configured yet. The Mailer reports an explicit error until a provider is added.

## Tests

Create a dedicated Postgres test database and set `TEST_DATABASE_URL` to it. The HTTP integration test creates a unique schema in that database and drops it when the test finishes. The database user needs permission to create and drop schemas.

```sh
TEST_DATABASE_URL='postgres://USER:PASSWORD@HOST:5432/cave_lms_test?sslmode=disable' go test ./...
```
