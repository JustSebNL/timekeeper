# TimeKeeper Store documentation

The Store is an optional distribution and entitlement service for TimeKeeper extensions. Core project tracking works without a Store account or network connection.

This repository contains the Store client and protocol documentation; the Store server is maintained in the separate private `TimeKeeper-Store` repository. The checked-in client default URL is the reserved placeholder `https://api.timekeeper.example`, not a live production service. A working deployment must provide its actual endpoint through `TIMEKEEPER_STORE_URL`.

The current client can authenticate, request entitlements, download and verify packages, and stage them locally. Staging is not activation: Core does not execute a staged package. The supervised plugin bridge and connector-specific capabilities remain gated. Do not treat Store or extension installation as a prerequisite for using local TimeKeeper.

The complete system blueprint—API, authentication, CRM, queues, connectors, logging, audit, security, and lifecycle—is [store-system-architecture.md](store-system-architecture.md).

## Documentation map

- [Developer guide](store-developer-guide.md) — build a plugin, package it, connect it through the external bridge, test it, and publish it without Core source changes.
- [API reference](store-api.md) — versioned routes, headers, authentication requirements, responses, and errors.
- [Authentication protocol](auth-protocol.md) — installation identity, sessions, refresh-token rotation, freshness, and secret boundaries.
- [Package format](package-format.md) — `.tkp` layout, manifest, checksums, detached Ed25519 signatures, download tickets, and extraction limits.
- [Extension model](extensions.md) — permission vocabulary and staging versus activation.
- [Two-box architecture](two-box-architecture.md) — Core/plugin isolation and `timekeeper-bridge/v1` control/capability tunnels.
- [Plugin architecture](plugin-architecture.md) — stable Core seam, including the legacy in-process development path and the supported external Store path.
- [Security scanning](plugin-security-scanning.md) — static destructive-code, dependency/SBOM, hermetic dynamic, quarantine, review, and rescan policy.

## Ownership boundaries

Core owns the stable capability contracts, launcher policy, permission grants, process supervision, lifecycle, and durable project state. The private Store owns accounts, entitlements, package metadata, signing keys, tickets, and publication. A plugin developer owns an implementation that fits an existing contract.

No plugin may edit Core source, import Core private packages, mutate the Core database, or widen its own permissions. A new capability requires a reviewed Core contract and release before a plugin can use it.

## Current implementation status

Implemented in the current Store/client codebase:

- Store v1 authentication and entitlement client/protocol;
- signed `.tkp` build and delivery contracts;
- Core checksum/signature verification;
- bounded extraction and manifest validation;
- plugin bridge handshake contract and refusal tests;
- staging-only CLI installation;
- developer and operator documentation.

Plugin activation is already represented by an open task in the current TimeKeeper project. Check that project tree before creating a duplicate core task from this status list.

Still gated deliberately:

- automatic execution of a staged package;
- generic trusted launcher mapping package identity to an executable;
- connector-specific capability adapters and schemas.

Those gates prevent a signed archive from becoming arbitrary Core code. The private Store deployment is separate from this repository; the checked-in `.example` URL is not a production endpoint.
