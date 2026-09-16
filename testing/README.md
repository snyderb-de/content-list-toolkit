# Testing

The repository now groups test support by feature instead of mixing everything into one shared folder.

- `testing/content-scan/` holds content-list fixtures, a fixture generator, and a feature-level runner.
- `testing/email-copy/` holds email-copy fixtures, a fixture generator, and a feature-level runner.
- `testing/manual-samples/` is for local real-world sample sets and stays ignored.
- `testing/manual-output/` is for local generated outputs and stays ignored.

Automated tests live close to the code: Go tests stay at the repo root in
`*_test.go`.

The tracked fixture folders under `testing/` are golden files for the Go
scanner and email-copy tests. They were originally the source of truth for
cross-language parity against the Python runtime; that runtime is retired, but
the fixtures kept their value as regression coverage and are still asserted
against by `scan_test.go` and `email_copy_test.go`.

`generate_fixture.py` in each folder regenerates them. Those scripts are
standard-library Python dev tooling and are unrelated to the retired runtime.
