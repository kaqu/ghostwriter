use crate::error::Result;
use crate::files::workspace::WorkspaceManager;
use ratatui::prelude::*;
use ratatui::widgets::{Block, Borders, List, ListItem, Paragraph};
use std::path::{Path, PathBuf};

#[allow(dead_code)]
#[derive(Debug, Clone)]
struct VisibleItem {
    name: String,
    path: PathBuf,
    is_dir: bool,
}

#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct FileNode {
    pub name: String,
    pub path: PathBuf,
    pub is_dir: bool,
    pub children: Vec<FileNode>,
    pub expanded: bool,
}

#[allow(dead_code)]
impl FileNode {
    fn load(ws: &WorkspaceManager, path: &Path) -> Result<Self> {
        let mut children = Vec::new();
        for entry in ws.list_dir(path)? {
            let child_path = path.join(&entry.name);
            if entry.is_dir {
                children.push(FileNode::load(ws, &child_path)?);
            } else {
                children.push(FileNode {
                    name: entry.name,
                    path: child_path,
                    is_dir: false,
                    children: Vec::new(),
                    expanded: false,
                });
            }
        }
        Ok(FileNode {
            name: path
                .file_name()
                .map(|s| s.to_string_lossy().into_owned())
                .unwrap_or_else(|| path.to_string_lossy().into_owned()),
            path: path.to_path_buf(),
            is_dir: true,
            children,
            expanded: true,
        })
    }

    fn gather(&self, out: &mut Vec<VisibleItem>, filter: &str) {
        if self.name.is_empty() || self.name.to_lowercase().contains(&filter.to_lowercase()) {
            out.push(VisibleItem {
                name: self.name.clone(),
                path: self.path.clone(),
                is_dir: self.is_dir,
            });
        }
        if self.expanded {
            for c in &self.children {
                c.gather(out, filter);
            }
        }
    }
}

#[allow(dead_code)]
#[derive(Debug)]
pub struct FilePicker {
    root: FileNode,
    pub search: String,
    pub selected: usize,
    visible: Vec<VisibleItem>,
}

#[allow(dead_code)]
impl FilePicker {
    pub fn new(ws: &WorkspaceManager) -> Result<Self> {
        let root = FileNode::load(ws, ws.root())?;
        let mut picker = Self {
            root,
            search: String::new(),
            selected: 0,
            visible: Vec::new(),
        };
        picker.update_visible();
        Ok(picker)
    }

    fn update_visible(&mut self) {
        let mut items = Vec::new();
        for c in &self.root.children {
            c.gather(&mut items, &self.search);
        }
        self.visible = items;
        if self.selected >= self.visible.len() {
            self.selected = self.visible.len().saturating_sub(1);
        }
    }

    pub fn set_search(&mut self, s: &str) {
        self.search = s.to_string();
        self.update_visible();
    }

    pub fn preview(&self) -> Result<String> {
        if let Some(node) = self.visible.get(self.selected) {
            if node.is_dir {
                return Ok(String::new());
            }
            let data = std::fs::read_to_string(&node.path)?;
            let preview: String = data.lines().take(10).collect::<Vec<_>>().join("\n");
            Ok(preview)
        } else {
            Ok(String::new())
        }
    }

    pub fn move_up(&mut self) {
        if self.selected > 0 {
            self.selected -= 1;
        }
    }

    pub fn move_down(&mut self) {
        if self.selected + 1 < self.visible.len() {
            self.selected += 1;
        }
    }
}

impl Widget for FilePicker {
    fn render(self, area: Rect, buf: &mut Buffer) {
        let block = Block::default().title("File Picker").borders(Borders::ALL);
        let inner = block.inner(area);
        ratatui::widgets::Widget::render(block, area, buf);
        let items: Vec<ListItem> = self
            .visible
            .iter()
            .map(|n| ListItem::new(n.name.clone()))
            .collect();
        let list =
            List::new(items).highlight_style(Style::default().add_modifier(Modifier::REVERSED));
        let chunks = Layout::default()
            .direction(Direction::Horizontal)
            .constraints([Constraint::Percentage(50), Constraint::Percentage(50)])
            .split(inner);
        ratatui::widgets::Widget::render(list, chunks[0], buf);
        let preview = Paragraph::new(self.preview().unwrap_or_default())
            .block(Block::default().borders(Borders::ALL).title("Preview"));
        preview.render(chunks[1], buf);
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ratatui::{Terminal, backend::TestBackend};
    use tempfile::tempdir;

    #[test]
    fn test_file_picker_overlay() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let picker = FilePicker::new(&ws).unwrap();
        let backend = TestBackend::new(20, 10);
        let mut terminal = Terminal::new(backend).unwrap();
        terminal
            .draw(|f| {
                #[allow(deprecated)]
                let area = f.size();
                f.render_widget(picker, area);
            })
            .unwrap();
        let size = terminal.backend().size().unwrap();
        assert_eq!(size.width, 20);
        assert_eq!(size.height, 10);
    }

    #[test]
    fn test_fuzzy_search_filtering() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("main.rs"), "fn main(){}\n").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let mut picker = FilePicker::new(&ws).unwrap();
        picker.set_search("main");
        assert!(picker.visible.iter().any(|n| n.name == "main.rs"));
    }

    #[test]
    fn test_file_preview() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("file.txt"), "line1\nline2\nline3\n").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let mut picker = FilePicker::new(&ws).unwrap();
        picker.set_search("file");
        let preview = picker.preview().unwrap();
        assert!(preview.contains("line1"));
    }

    #[test]
    fn test_keyboard_navigation() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("a.txt"), "").unwrap();
        std::fs::write(dir.path().join("b.txt"), "").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let mut picker = FilePicker::new(&ws).unwrap();
        let initial = picker.selected;
        picker.move_down();
        assert_eq!(picker.selected, initial + 1);
        picker.move_up();
        assert_eq!(picker.selected, initial);
    }
}
