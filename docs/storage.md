# Storage

goose persists sessions in SQLite. It does not encode messages as vectors.

## SQLite

`SessionManager` opens one SQLite pool through `sqlx` (`crates/goose/src/session/session_manager.rs`). Journal mode is WAL. The file is the sessions database reported by `goose info` as "Sessions DB (sqlite)".

Tables created in `create_schema`:

- `schema_version`
- `sessions`
- `messages`, with indexes on `session_id`, `timestamp`, and `message_id`

Message bodies are JSON (`content_json`, `metadata_json`). Migrations run when `schema_version` already exists. First start imports legacy on-disk sessions into this database.

`sqlx` appears only on this session pool (and tests that open the same kind of pool). Workspace `Cargo.toml` files do not depend on an embedding, vector-index, or vector-database crate.

## Chat search

`ChatHistorySearch` (`crates/goose/src/session/chat_history_search.rs`) splits the query on whitespace, wraps each word as `%word%`, and matches with `LOWER(json_extract(...)) LIKE ?` against text parts of `messages.content_json`. Results join back to `sessions`. This is the chatrecall path.

There is no embedding step, no vector column, and no FTS index on that query.

## Other persistence

- Config is YAML via `crates/goose/src/config/base.rs`.
- Permissions are a file map (`config/permission.rs`), not a database.
- Secrets use the system keyring when the `system-keyring` feature is on, with a file fallback.

## Embedding names in the catalog

`canonical_mapping_report.json` lists embedding model ids (`text-embedding-3-*`, `gemini-embedding-001`, and similar) so chat-model canonicalization can recognize them. `canonical.rs` excludes names containing `embedding` from the chat catalog. That data is a name list. goose does not call those models to embed session text.

Databricks provider tests include fixture payloads whose `supported_api_types` mention embeddings. Those tests assert model-list filtering. They do not run an embedding request.
