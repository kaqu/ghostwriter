# Project Agents Guide

This file defines rules for AI agents working in this repository.

## Reference Documents
- `PRD.md` describes the application requirements and architecture.
- `PLAN.md` lists implementation plan with acceptance criteria.
- `TODO.md` lists implementation tasks with current status.

## Commit Guidelines
- Use **Conventional Commits** style messages (`feat:`, `fix:`, `docs:`, `chore:`...).
- Keep pull requests focused on a single task from `PLAN.md`.

## Development Workflow
1. Format code before committing
2. Run linters and unit tests

## Rust Edition
- The workspace targets **Rust 2024** using the stable toolchain. New crates
  should set `edition.workspace = true` to inherit this edition.

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

## Test coverage
- Every change must include unit tests exercising all new logic
- Strive for 100% coverage on modified code using `cargo tarpaulin`
- Run `cargo tarpaulin` locally and ensure coverage remains complete

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
