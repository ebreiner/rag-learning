---
type: decision
title: config.yaml with data sources
status: accepted
date: 2026-10-17
tags: [config, data-source, collections, metadata, sidecar]
---

# 0001 config.yaml with data sources

Supersedes these parts of [0000](0000-migrated-baseline.md): Config ("no config file"), Collections (collection name and weight from CLI flags).

## Context

- [Handover](../requirements/rag-cli-handover.md) RC10, RC14, RC17, RC18: many data sources, each with attributes, URL template and freshness policy. Sidecars for each file. Required classification.
- More than one installation uses rag-cli. All differences between installations must go into `config.yaml`, not into code.
- Now `extract` gets `--collection-name` and `--collection-weight` for each run. The collection has no description and no attributes.

## Options considered

1. Keep CLI flags and add more flags. A run for each data source. The installation config is then in shell scripts.
2. Env variables. Not usable for nested data (list of data sources with attributes).
3. One `config.yaml` for each installation. Accepted.

## Decision

- `config.yaml` is the installation config. It lists the data sources.
- A data source is a collection. Fields: name, description, recursive directory, attributes, optional URL template, freshness policy, optional weight.
- Data source attributes are defaults for each document of the data source. A sidecar `<file>.meta.json` overrides them for its file.
- System-controlled attributes cannot be overridden.
- rag-cli defines a required attribute set. The RC18 classification attributes are in it. Each document must get each required value from its sidecar or from an explicit data source attribute. rag-cli has no built-in default for them. A document without a valid value is rejected with a hard error.
- rag-cli defines the names and the validation of the known meanings (RC17).
- The schema, the precedence between flags and file, and the full required set go into a data source design.

## Consequences

- `--collection-name` and `--collection-weight` go away.
- The DB must store document metadata and data source fields (description, attributes).
- A config change needs a restart of `serve` (see [0004](0004-mcp-tool-surface.md)).
- The open question in the old backlog about the weight on an `extract` re-run is obsolete. The weight comes from the config.
