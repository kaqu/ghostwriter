use std::fs;

#[test]
fn release_workflow_has_write_permission() {
    let yaml =
        fs::read_to_string("../.github/workflows/release.yml").expect("release workflow not found");
    assert!(
        yaml.contains("permissions:") && yaml.contains("contents: write"),
        "release workflow must grant write permissions to contents"
    );
}
