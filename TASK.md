# TASK

## Scope
Harden runtime behavior and test hygiene for this repository.

## Constraints
- No git commits or tags from subprocesses unless explicitly requested.
- Keep changes minimal, testable, and production-safe.
- Prefer deterministic shutdown/startup behavior.

## Required Output
- Small PR-sized patch.
- Repro steps.
- Validation commands and expected results.
- Known risks/limits.

## Priority Tasks
1. Implement true delete behavior for discovered devices/entities.
2. Handle retained discovery removal and state reconciliation.
3. Avoid in-memory rediscovery ghosts after API deletion.
4. Keep discovery topic fully env-driven and test-safe.

## Done Criteria
- Deleting a discovered device keeps it deleted unless rediscovered from live MQTT.
- Test artifacts cannot repopulate after cleanup + restart.

## Validation Checklist
- [ ] Build succeeds for this repo.
- [ ] Local targeted tests (if present) pass.
- [ ] No new background orphan processes remain.
- [ ] Logs clearly show failure causes.
