# Data Model: TUI Command & Keybinding Cleanup

## Existing Entities (no changes)

### Connection
Defined in `internal/config/connection.go:180-208`. Relevant fields:
- `Group string` — references a group by name
- `Name string` — display name, used for `:mv <name> <group>` lookup

### Group
Defined in `internal/config/connection.go:224-227`:
- `Name string` — unique identifier
- `Color string` — optional display color

### Config
Defined in `internal/config/config.go:94-102`:
- `Groups []Group` — explicit group list (persisted to YAML)
- `Connections []Connection` — connection list

## New Methods on Config

### AddGroup(name string) error
- Validates name is non-empty
- Checks no existing group with same name (case-sensitive)
- Appends `Group{Name: name}` to `Config.Groups`
- Returns error if group already exists

### DeleteGroup(name string) error
- Checks group exists in `Config.Groups`
- Checks no connections have `Group == name`
- Removes group from `Config.Groups`
- Returns error if group not found or has connections

### FindGroup(name string) (*Group, bool)
- Iterates `Config.Groups`
- Returns pointer to group and true if found, nil and false otherwise

### HasConnectionsInGroup(name string) bool
- Iterates `Config.Connections`
- Returns true if any connection has `Group == name`

### MoveConnection(connID string, targetGroup string) error
- Finds connection by ID
- Updates `Connection.Group` to targetGroup
- If targetGroup not in `Config.Groups`, auto-adds it via `AddGroup`
- Returns error if connection not found

## Removed Entities

### MarkManager (DELETE — marks.go)
- `Mark` struct (view, cursor)
- `MarkManager` struct (marks map, jumpList, jumpIndex)
- All methods: SetMark, GetMark, AllMarks, PushJump, JumpBack, JumpForward, LastJump

### TreeModel (DELETE — tree.go)
- `TreeNode` struct
- `TreeEntry` struct
- `TreeModel` struct
- All methods: Toggle, Expand, Collapse, ExpandAll, CollapseAll, Flatten, View, etc.

## Modified Entities

### App struct (app.go)
Fields to remove:
- `marks *MarkManager` (line 70)
- `markPending rune` (line 71)
- `tree *TreeModel` (line 57)
- `viewMode string` (line 84)
- `treeFoldPending bool` (line 85)

### MotionEngine (motion.go)
- Remove `OpChange` constant from operator types
- Remove change-related handling from operator dispatch

### CommandEngine registry (command.go)
Commands to remove from `defaultCommands()`:
- disconnect/dc (line 39)
- theme (line 48)
- version/ver (line 59)
- marks (line 60)
- template/tpl (line 61)
- settings/set (line 62)
- recordings/rec (line 63)
- health (line 70)
- vault (line 72)

### Leader menu (leader.go)
Items to remove:
- Groups: list-groups, filter-by-group (replace with working items)
- View: view-tree
- Health: check-selected
- Options: theme

Items to add:
- Connections group: add, edit, delete
- Find group: frequent, favorites, all
- Groups group: create, delete, move, filter
- Data group (new): note, field
