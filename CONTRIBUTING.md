# Contributing

Thank you for helping improve Home Lab Observer.

## Before coding

1. Read the [constitution](.specify/memory/constitution.md), [repository instructions](AGENTS.md), and [coding standards](docs/coding-standards.md).
2. Use an existing accepted specification or propose a focused specification with user journeys, requirements, non-goals, acceptance criteria, plan, and tasks.
3. Add or update an ADR before implementing a durable architecture change.

## Pull requests

- Keep each pull request to one coherent specification slice.
- Include the requirement/task identifiers it satisfies and the commands/evidence used to verify them.
- Use synthetic data and reserved domains in examples and tests.
- Do not include logs or screenshots containing real hostnames, addresses, usernames, credentials, or private repository details.
- Prefer squash auto-merge after required checks and review pass.

## Local checks

Run:

```powershell
./scripts/check-repository.ps1
```

Language, API, UI, and packaging checks will be added by their implementing specifications.
