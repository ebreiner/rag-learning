---
type: decision
title: One config file, values go into adapters at the composition root
status: accepted
date: 2026-10-19
tags: [config, cli, ingest, serve, composition-root, secrets]
---

# 0011 One config file, values go into adapters at the composition root

Adds to [0001](0001-config-yaml-data-sources.md) and [0006](0006-ingest-command.md). Supersedes this part of [0006](0006-ingest-command.md): "The step commands stay for development for now."

## Context

- [0001](0001-config-yaml-data-sources.md) decides on a `config.yaml`, but it does not say how the values reach the code.
- Before this change, each command had its own CLI flags and env variables. `extract`, `chunk` and `embed` were separate commands. The fusion weights came from env variables.
- The steps (`internal/<stage>/step`) talk to the outside only through ports. A step must not know the config file.

## Options considered

1. Each step port owns its config section, and the step gets its config values.
2. The config goes only into the adapter constructors at the composition root. A step gets behaviour through its ports. Accepted.
3. Keep CLI flags and add a config file as a second input with a precedence order. Two inputs for one value make errors hard to find.

## Decision

- **Commands:** `ingest`, `serve`, `eval` and `retrieve`. `ingest` runs extract, chunk and embed in sequence on one DB connection. The commands `extract`, `chunk`, `embed` and `inspect` are removed.
- **One input:** one YAML file with the sections `storage`, `log`, `docling`, `embedding`, `sources`, `retrieval`, `server` and `eval`. The global flag `--config` / `-c` gives its path. Commands have no other flags for settings. A flag stays only for a choice for each run, for example `eval --baseline` and `retrieve --query`.
- **Two representations:** a private `rawConfig` with the YAML tags, and a public `Config` with typed values. Only `config.Load` makes a `Config`. Its steps are parse, resolve and validate.
- **Load rules:**
  - Unknown keys are an error.
  - Relative paths resolve against the directory of the config file.
  - URLs are parsed into `*url.URL`.
  - Durations are strings with a unit.
  - `validate` collects all errors with `errors.Join`. Each error has the YAML path as a prefix.
  - There are no built-in defaults for the values in the file.
- **Ownership:** the config package owns the config types. The composition root (`cmd/cli/cmd`, `cmd/cli/cmd/ingest`, `cmd/cli/cmd/serve`) reads `Config` and gives the values to the adapter constructors. The domain owns a value only when step code reads it, for example `retrievalstep.FusionConfig`.
- **Adapter selection:** a factory at the composition root selects the adapter from a config enum, for example `NewEmbedClient` with `embedding.backend` `openai` or `kreuzberg`.
- **No hot reload:** after `ingest` or a config change, restart `serve`. There is no DB swap at runtime.
- **Secrets:** for now, secrets such as docling headers go into the config file. `config.yaml` is gitignored. `config.yaml.example` is committed and has no secrets. The MCP token stays in the env `RAG_CLI_API_TOKEN` until the OAuth work ([0005](0005-oauth-resource-server.md)).

## Consequences

- An installation is one file. A change of an installation does not need a code change.
- The `justfile` recipes use only `-c` and the run choices.
- `ingest` skips work that is done: extract skips a known content `sha256`, chunk skips a chunked document, embed skips a chunk with a vector. The add, replace and delete logic of [0006](0006-ingest-command.md) is not built yet.
- A secret in a plain file is acceptable only for development. Revisit this with the OAuth work.
- Evidence: the eval run `eval/runs/20261019T124955Z.json` on branch `server_and_config_refactor` (commit `5e051ea` plus the uncommitted `eval` and `ingest` changes) has the same metrics as the baseline `20261018T094013Z.json`. The `golden_sha256` and `chunk_count` (1116) are also the same.
