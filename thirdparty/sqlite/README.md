# SQLite

This directory vendors the SQLite 3.53.4 amalgamation and the Go bindings copied from the local `hr-sama` project.

The Go package is `rheincni/thirdparty/sqlite/bindings`. It exposes both the low-level binding and a `database/sql` connector through `OpenDatabaseSQL`.

SQLite's license is in `LICENSE.md`.
