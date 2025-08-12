use ghostwriter_client::local::LocalClient;
use std::io::{Read, Write};
use tempfile::NamedTempFile;

#[tokio::test]
async fn open_insert_save() {
    let mut file = NamedTempFile::new().unwrap();
    write!(file, "hi").unwrap();
    let path = file.path().to_path_buf();

    let mut client = LocalClient::open(path.clone(), 80, 24).unwrap();

    // initial frame
    let frame = client.request_frame().await;
    assert_eq!(frame.lines[0].text, "hi");

    client.insert(" there").await;
    let frame = client.next_frame().await;
    assert_eq!(frame.lines[0].text, " therehi");

    client.save().await;
    let _ = client.request_frame().await; // ensure save processed

    let mut contents = String::new();
    std::fs::File::open(&path)
        .unwrap()
        .read_to_string(&mut contents)
        .unwrap();
    assert_eq!(contents, " therehi");
}
