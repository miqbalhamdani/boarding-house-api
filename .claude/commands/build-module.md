# Build Module Workflow

Input:
- Module name: $ARGUMENTS

Workflow:
1. Use senior-backend skill to implement the selected module.
2. If database schema changes are needed, create or update migration files.
3. Run the migration process locally against the development database.
4. Verify migration status and confirm the database schema matches the module requirements.
5. After coding and migration finish, use code-reviewer skill to review.
6. If blocking issues are found, fix them before continuing.
7. Use senior-qa skill to add and run tests.
8. If QA fails, fix issues before continuing.
9. Create or update Bruno API specs.
10. Run lint/test/build.
11. Commit and push only if all gates pass.

Required docs:
- /docs/product-requirements.md
- /docs/business-rules.md
- /docs/database-schema.md
- /docs/api-spec.md
- /docs/coding-rules.md
- relevant /docs/modules/*.md file

Quality gates:
- Code review must pass.
- QA must pass.
- Tests must pass.
- Lint/build must pass if configured.
- Bruno specs must be updated.
- Do not commit if any gate fails.

Git:
- Run git status before commit.
- Commit only related files.
- Commit message format:
  feat(<module>): implement <module name>
- Push to current branch.

Final output:
- Module built
- Skills used
- Files changed
- Endpoints implemented
- Bruno specs updated
- Tests run
- Review result
- Commit hash
- Branch name
- Remaining TODOs