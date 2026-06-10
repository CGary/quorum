# Skill Registry - hsme

## User Skills
| Skill | Trigger | Path |
|-------|---------|------|
| go-testing | Go tests, Bubbletea TUI testing | /home/gary/.agents/skills/go-testing/SKILL.md |
| branch-pr | Creating a pull request | /home/gary/.agents/skills/branch-pr/SKILL.md |
| issue-creation | Creating a GitHub issue | /home/gary/.agents/skills/issue-creation/SKILL.md |

## Project Skills
| Skill | Trigger | Path |
|-------|---------|------|
| spec-kitty.implement | Execute a work package implementation | /home/gary/dev/hsme/.agents/skills/spec-kitty.implement/SKILL.md |
| spec-kitty.review | Review a work package implementation | /home/gary/dev/hsme/.agents/skills/spec-kitty.review/SKILL.md |
| spec-kitty.tasks | Break a plan into work packages | /home/gary/dev/hsme/.agents/skills/spec-kitty.tasks/SKILL.md |

## Compact Rules

### go-testing
- Use standard library `testing` package.
- Prefer table-driven tests for multiple cases.
- Use `t.Parallel()` when tests are independent.

### spec-kitty.implement
- Follow the plan exactly.
- Verify each task with tests.
- Maintain consistency with existing architecture.
