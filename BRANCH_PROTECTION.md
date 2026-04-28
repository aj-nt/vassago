# Branch Protection: mnemosyne

## Settings (configure in GitHub repo Settings → Branches → Branch protection rules)

### Branch: `main`

- ✅ Require a pull request before merging
  - Require approvals: 1 (AJ)
  - Dismiss stale reviews on push: yes
- ✅ Require status checks to pass before merging
  - Status checks required:
    - `test` (from ci.yml)
    - `lint` (from ci.yml)
    - `security` (from ci.yml)
    - `qa-pass` (from qa-gate.yml)
- ✅ Require branches to be up to date before merging
- ✅ Require linear history (no merge commits)
- ❌ Do NOT require signed commits (we sign via SSH on the Mac)
- ❌ Do NOT require conversation resolution (we're fast-moving)

### Branch: `dogfood`

Same as `main` — this is the fork's integration branch.

## Workflow

```
main (protected)
  ↑
  PR (requires: CI green + AJ approval)
  ↑
phase-2/agent-core  ← work happens here
phase-3/telepathy    ← or here
```

1. Create a branch: `git checkout -b phase-2/agent-core`
2. Work, commit, push frequently — CI runs on every push
3. When the phase is code-complete:
   - Run the Manual QA Pass locally: `make test-race`
   - Run the Manual Red Team Pass (try to break it)
   - Run the Refactoring Pass (Fowler discipline)
4. Create PR to `main`
5. CI must be green (automated)
6. AJ approves (manual)
7. Merge

## Exceptions

- Hotfixes to `main` can bypass the PR process for critical fixes, but must still pass CI
- Prototype branches (not merging to `main`) don't need any gates