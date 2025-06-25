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
    ws: WorkspaceManager,
    root: FileNode,
    pub search: String,
    pub selected: usize,
    visible: Vec<VisibleItem>,
}

#[allow(dead_code)]
impl FilePicker {
    pub fn new(ws: WorkspaceManager) -> Result<Self> {
        let root = FileNode::load(&ws, ws.root())?;
        let mut picker = Self {
            ws,
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

    fn find_mut<'a>(node: &'a mut FileNode, path: &Path) -> Option<&'a mut FileNode> {
        if node.path == path {
            return Some(node);
        }
        for child in &mut node.children {
            if let Some(f) = Self::find_mut(child, path) {
                return Some(f);
            }
        }
        None
    }

    pub fn toggle_expand(&mut self) {
        if let Some(item) = self.visible.get(self.selected) {
            if let Some(node) = Self::find_mut(&mut self.root, &item.path) {
                if node.is_dir {
                    node.expanded = !node.expanded;
                    self.update_visible();
                }
            }
        }
    }

    pub fn create_file(&mut self, name: &str) -> Result<()> {
        let dir = self
            .visible
            .get(self.selected)
            .map(|n| {
                if n.is_dir {
                    n.path.clone()
                } else {
                    n.path.parent().unwrap().to_path_buf()
                }
            })
            .unwrap_or_else(|| self.ws.root().to_path_buf());
        self.ws.create_file(&dir.join(name))?;
        self.root = FileNode::load(&self.ws, self.ws.root())?;
        self.update_visible();
        Ok(())
    }

    pub fn create_dir(&mut self, name: &str) -> Result<()> {
        let dir = self
            .visible
            .get(self.selected)
            .map(|n| {
                if n.is_dir {
                    n.path.clone()
                } else {
                    n.path.parent().unwrap().to_path_buf()
                }
            })
            .unwrap_or_else(|| self.ws.root().to_path_buf());
        self.ws.create_dir(&dir.join(name))?;
        self.root = FileNode::load(&self.ws, self.ws.root())?;
        self.update_visible();
        Ok(())
    }

    pub fn rename_selected(&mut self, new_name: &str) -> Result<()> {
        if let Some(item) = self.visible.get(self.selected) {
            let new_path = item.path.parent().unwrap().join(new_name);
            self.ws.rename(&item.path, &new_path)?;
            self.root = FileNode::load(&self.ws, self.ws.root())?;
            self.update_visible();
        }
        Ok(())
    }

    pub fn delete_selected(&mut self) -> Result<()> {
        if let Some(item) = self.visible.get(self.selected) {
            self.ws.delete(&item.path)?;
            self.root = FileNode::load(&self.ws, self.ws.root())?;
            self.update_visible();
        }
        Ok(())
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
        let picker = FilePicker::new(ws).unwrap();
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
        let mut picker = FilePicker::new(ws).unwrap();
        picker.set_search("main");
        assert!(picker.visible.iter().any(|n| n.name == "main.rs"));
    }

    #[test]
    fn test_file_preview() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("file.txt"), "line1\nline2\nline3\n").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let mut picker = FilePicker::new(ws).unwrap();
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
        let mut picker = FilePicker::new(ws).unwrap();
        let initial = picker.selected;
        picker.move_down();
        assert_eq!(picker.selected, initial + 1);
        picker.move_up();
        assert_eq!(picker.selected, initial);
    }

    #[test]
    fn test_expand_and_file_operations() {
        let dir = tempdir().unwrap();
        std::fs::create_dir(dir.path().join("sub")).unwrap();
        std::fs::write(dir.path().join("sub/file.txt"), "").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let mut picker = FilePicker::new(ws).unwrap();
        picker.toggle_expand();
        assert!(picker.visible.len() > 0);
        picker.create_file("new.txt").unwrap();
        assert!(std::fs::metadata(dir.path().join("sub/new.txt")).is_ok());
        picker.selected = picker
            .visible
            .iter()
            .position(|n| n.name == "new.txt")
            .unwrap();
        picker.rename_selected("renamed.txt").unwrap();
        assert!(std::fs::metadata(dir.path().join("sub/renamed.txt")).is_ok());
        picker.selected = picker
            .visible
            .iter()
            .position(|n| n.name == "renamed.txt")
            .unwrap();
        picker.delete_selected().unwrap();
        assert!(std::fs::metadata(dir.path().join("sub/renamed.txt")).is_err());
    }
}
