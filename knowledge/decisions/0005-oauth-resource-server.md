---
type: decision
title: OAuth resource server, static token for development
status: accepted
date: 2026-10-17
tags: [serve, auth, oauth, telemetry]
---

# 0005 OAuth resource server, static token for development

Supersedes this part of [0000](0000-migrated-baseline.md): Serve and MCP (`BearerAuth` with the static token as the only mode). Also Config ("the only env input is `RAG_CLI_API_TOKEN`").

## Context

- [Handover](../requirements/rag-cli-handover.md) RC19a, RC19b, RC19d: OAuth resource server as the MCP spec requires, identity in telemetry, static token for development.
- Now `BearerAuth` compares one static token. OpenWebUI sends it through a static header.

## Options considered

1. Static token only. Does not satisfy RC19a and RC19b.
2. An auth proxy in front of `serve`. The Protected Resource Metadata belongs to the MCP server, and RC19b needs the identity inside rag-cli.
3. Resource server inside `serve`. Accepted.

## Decision

- `serve` checks JWTs from an OIDC issuer set in the config (Keycloak), with its JWKS.
- `serve` publishes Protected Resource Metadata (RFC 9728).
- A request without a valid token gets 401 with the metadata location in `WWW-Authenticate`.
- rag-cli issues no tokens and keeps no users.
- The identity from the token goes into the span of each tool call.
- The static token stays as a config mode for development.

## Consequences

- The PO supplies a test Keycloak realm (handover §6).
- Client registration (CIMD or DCR) is between the client and Keycloak. It is not in rag-cli.
