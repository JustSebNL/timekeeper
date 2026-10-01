# Administrator and security groups

TimeKeeper includes a local Administrator view for managing the locally observed identities, groups, and permissions. The API enforces those permission checks; hiding a dashboard button is not treated as authorization.

This is **not an authenticated account system**. The local server binds to loopback and trusts the operating-system user boundary. `X-Agent-ID` is an attribution claim that any local process can supply; it is not proof of identity. Do not expose this service to other machines or rely on these groups as enterprise or multi-user authentication.

## Built-in groups

- **Administrators** — access to the administrator catalog and workspace permissions.
- **Operators** — manage projects, messages, and Sprint execution without user/group administration.
- **Agents** — read projects, write project memory, execute Sprints, and read usage.
- **Viewers** — read-only project, memory, and usage access.
- **Auditors** — read-only operational, usage, diagnostic, and administrator audit evidence.

Administrators can create additional groups, choose permissions, and assign locally observed users to groups. Built-in groups are seeded during database migration and are not deleted from the dashboard.

## Permission shape

Permissions use stable `resource.action` identifiers, for example:

```text
admin.users.read
admin.users.manage
admin.groups.read
admin.groups.manage
admin.audit.read
projects.read
projects.write
messages.read
messages.write
sprints.execute
usage.read
diagnostics.read
visibility.project.private_context.read
visibility.execution.history.read
visibility.usage.tokens.read
visibility.diagnostics.internal_error.read
visibility.diagnostics.stack_trace.read
```

The server calculates effective permissions from enabled local users and group membership. A disabled account receives no administrator permissions, and the final enabled Administrator cannot be disabled or removed from the Administrator group.

## Local bootstrap boundary

The first stable `X-Agent-ID` identity that calls `/api/v1/admin/me` is bootstrapped as the first Administrator; later unknown local identities are created as Viewers. Identity-aware CLI builds may supply an automatically resolved FASE ID. Direct local API clients can still choose their own `X-Agent-ID`, so this bootstrap is useful for local organization only—not authentication.

## Audit trail

Administrator mutations and denied administrator access are recorded in `admin_audit_events` with actor, action, target, request ID, client ID, surface, and bounded JSON details. Sensitive request bodies and credentials are not recorded.

For the full local security boundary, see [THREAT_REVIEW.md](../THREAT_REVIEW.md). Administrator API routes are listed in [API.md](../API.md).
