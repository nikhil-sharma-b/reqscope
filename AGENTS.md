# Reqscope agent instructions

## Git branch naming convention

When creating feature or issue branches, use a milestone or topic prefix:

- Format: `<milestone-or-topic>/<feature-name>`.
- Use kebab-case for both parts.
- Choose the prefix for the milestone or topic and a descriptive feature name.

Examples:

- `reqscope-v1/initialize-project`
- `reqscope-v1/swagger-url-import`
- `reqscope-v1/response-comparison`

Source: [Velura branch naming convention](https://github.com/nikhil-sharma-b/velura/blob/dev/AGENTS.md#git-branch-naming-convention).

## Commit message convention

Use a single-line Conventional Commit message:

- Format: `<type>[optional scope][!]: <description>`.
- Types: `feat`, `fix`, `docs`, `refactor`, `test`, `build`, `ci`, `perf`, `style`, or `chore`.
- Use an optional scope to identify the affected area and `!` for a breaking change.
- Write a concise, imperative description without a trailing period.

Examples:

- `feat(import): discover specifications from Swagger UI URLs`
- `fix(auth): reject unauthorized redirect destinations`
- `docs: document agent Git conventions`
