# Git Commit Message Convention

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification.

## Format

```text
<type>(<scope>): <subject>

<body>

<footer>
```

## Types

*   `feat`: A new feature
*   `fix`: A bug fix
*   `docs`: Documentation only changes
*   `style`: Changes that do not affect the meaning of the code (white-space, formatting, missing semi-colons, etc)
*   `refactor`: A code change that neither fixes a bug nor adds a feature
*   `perf`: A code change that improves performance
*   `test`: Adding missing tests or correcting existing tests
*   `chore`: Changes to the build process or auxiliary tools and libraries such as documentation generation

## Scopes

*   `agent`: Internal agent logic
*   `controller`: Kubernetes controller logic
*   `api`: CRD definitions
*   `tools`: Diagnosis tools
*   `llm`: LLM integration
*   `docs`: Documentation files

## Example

```text
feat(agent): implement light checkpoint mechanism

Add Restore method to BaseAgent to support crash recovery.
Update engine loop to trigger OnStepComplete callback.

Closes #123
```
