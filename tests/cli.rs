use assert_cmd::Command;
use predicates::prelude::*;

#[test]
fn shows_help() {
    Command::cargo_bin("ghostwriter")
        .unwrap()
        .arg("--help")
        .assert()
        .success()
        .stdout(predicate::str::contains("Usage"));
}

#[test]
fn shows_version() {
    Command::cargo_bin("ghostwriter")
        .unwrap()
        .arg("--version")
        .assert()
        .success()
        .stdout(predicate::str::contains(env!("CARGO_PKG_VERSION")));
}
