# Contributing

Kronos follows the task order in [.project/TASKS.md](.project/TASKS.md).
Keep changes small, covered by tests, and aligned with the dependency policy in
[.project/PROMPT.md](.project/PROMPT.md).

Before sending a change, run:

```bash
make check
```

Use conventional commit messages and include the task ID when a change completes
an item from the task list, for example:

```text
feat(core): add UUIDv7 IDs
```
