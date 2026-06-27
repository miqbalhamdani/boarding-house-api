# Build Module Workflow

Input:

* Module name: $ARGUMENTS

Workflow:
1. Use senior-backend skill to implement the selected module.
2. If database schema changes are needed, create or update migration files.
3. Run the migration process locally against the development database.
4. Verify migration status and confirm the local database schema matches the module requirements.
5. After coding and migration finish, use code-reviewer skill to review.
6. If blocking issues are found, fix them before continuing.
7. Use senior-qa skill to add and run tests.
8. If QA fails, fix issues before continuing.
9. Create or update Bruno API specs.
10. Run lint/test/build.
11. Commit changes only if all gates pass.
12. Push the branch to remote.
13. Create a pull request.
14. Share the pull request link.

Required docs:

* /docs/product-requirements.md
* /docs/business-rules.md
* /docs/database-schema.md
* /docs/api-spec.md
* /docs/coding-rules.md
* relevant /docs/modules/*.md file

Migration:

* Check whether the selected module requires database changes.
* If database changes are required, create a proper migration file.
* Migration must match /docs/database-schema.md.
* Run migration locally before code review and QA.
* Verify migration status after running migration.
* If migration fails, fix it before continuing.
* Include migration files in the final changed files summary.

Quality gates:

* Code review must pass.
* QA must pass.
* Tests must pass.
* Local migration must run successfully if database changes exist.
* Lint/build must pass if configured.
* Bruno specs must be updated.
* Do not commit if any gate fails.
* Do not create a pull request if any gate fails.

Git:

* Use the GitHub CLI (`gh`) for ALL GitHub operations (PRs, issues, API). Use `git` only for local/commit/push plumbing.
* Run git status before commit.
* Commit only related files.
* Commit message format:
  feat(<module>): implement <module name>
* Push to current branch after commit:
  `git push -u origin HEAD`

Pull Request:

* Running this workflow is explicit authorization to push and open the PR automatically — do NOT stop to ask for confirmation.
* After the branch is pushed, create the pull request automatically with `gh`:
  `gh pr create --base main --head <current-branch> --title "<title>" --body "<body>"`
* If a PR already exists for the branch, update it instead: `gh pr edit <number-or-branch> --title ... --body ...`.
* Pull request title format:
  feat(<module>): implement <module name>
* Pull request description must include:

  * module built
  * summary of changes
  * endpoints implemented
  * migration files changed
  * review result
* Retrieve and share the PR URL with `gh pr view --json url -q .url` in the final output.
* If `gh` is not authenticated, stop and report `gh auth login` is required (do not fall back to manual web links).

Final output:

* Module built
* Skills used
* Files changed
* Migration files changed
* Local migration command run
* Local migration result
* Endpoints implemented
* Bruno specs updated
* Tests run
* Review result
* Commit hash
* Branch name
* Pull request link
* Remaining TODOs
