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
The repo includes a `rust-toolchain.toml` file which installs the `rustfmt` and
`clippy` components automatically. To format the code, navigate to the
`ghostwriter` directory and run:
```bash
cargo fmt
```

## Running linter
To lint the code, navigate to the `ghostwriter` directory and run:
```bash
cargo clippy -- -D warnings
```

## Running tests
To run tests, navigate to the `ghostwriter` directory and run:
```bash
cargo test
```
Our GitHub CI also runs `cargo test` automatically for every pull request.

## Running a single test
To execute a specific test, pass its full module path to `cargo test`:
```bash
cargo test path::to::module::test_name
```
Use quotes if the test name contains spaces or special characters. You can also
use `cargo test -- --ignored` to run tests marked with `#[ignore]`.

## Rust coding best practices
- Keep functions small and focused on one task
- Prefer immutable variables (`let` over `let mut`) when possible
- Use `Result` and `Option` for error handling instead of panics
- Document public APIs with `///` comments
- Run `cargo fmt` and `cargo clippy` to maintain consistent style and catch
  common mistakes

Update this file with additional rules (tests, linting, style) as the project grows.
