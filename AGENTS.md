# Project Agents Guide

This file defines rules for AI agents working in this repository.

## Reference Documents
- `PRD.md` describes the application requirements and architecture.
- `PLAN.md` lists implementation tasks with acceptance criteria.

## Commit Guidelines
- Use **Conventional Commits** style messages (`feat:`, `fix:`, `docs:`, `chore:`...).
- Keep pull requests focused on a single task from `PLAN.md`.

## Development Workflow
1. Format code before committing
2. Run linters and unit tests

## Running formatter
To format the code, navigate to the `ghostwriter` directory and run:
```bash
cargo fmt
```

## Running linter
To lint the code, navigate to the `ghostwriter` directory and run:
```bash
cargo clippy
```

## Running tests
To run tests, navigate to the `ghostwriter` directory and run:
```bash
cargo test
```
Our GitHub CI also runs `cargo test` automatically for every pull request.

Update this file with additional rules (tests, linting, style) as the project grows.
