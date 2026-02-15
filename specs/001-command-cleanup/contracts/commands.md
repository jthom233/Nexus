# Command Contracts: Post-Cleanup Registry

## Commands to REMOVE from registry

| Name | Aliases | Reason |
|------|---------|--------|
| disconnect | dc | Stub — sessions view covers this |
| theme | — | Stub — no theme system |
| version | ver | Stub — no version constant |
| marks | — | Stub — removing marks system |
| template | tpl | Stub — no command handler |
| settings | set | Stub — no settings UI |
| recordings | rec | Stub — no recording system |
| health | — | Stub — redundant with `:pulse` and `r` key |
| vault | — | Stub — no vault UI |

## Commands to KEEP (verified working)

| Name | Aliases | ArgSpec |
|------|---------|---------|
| connect | — | `<name\|user@host>` |
| quit | q | — |
| add | — | — |
| edit | — | `<name>` |
| delete | del, rm | `<name>` |
| sort | — | `<field>` |
| filter | — | `[-t tag] [-g group] [pattern]` |
| tag | — | `<add\|remove> <tag>` |
| group | — | `[name]` |
| help | — | — |
| export | — | `<csv\|json\|yaml> <path>` |
| import | — | `<csv\|json> <path>` |
| import-ssh | — | — |
| favorites | fav | — |
| recent | — | — |
| frequent | — | — |
| pulse | — | — |
| log | logs, audit | — |
| sessions | — | — |
| note | — | `<text>` |
| field | — | `set <key> <value> \| remove <key>` |
| all | — | — |

## Commands to IMPLEMENT

### `:mkdir <name>`
- **Input**: Group name (required, non-empty)
- **Behavior**: Create group via `Config.AddGroup(name)`
- **Success**: Flash "Group {name} created"
- **Error**: Flash "Group {name} already exists"
- **Undo**: Not undoable

### `:rmdir <name>`
- **Input**: Group name (required)
- **Behavior**: Delete group via `Config.DeleteGroup(name)`
- **Success**: Flash "Group {name} removed"
- **Error**: Flash "Group {name} not found" or "Group {name} has connections"
- **Undo**: Not undoable

### `:mv [connection] <group>`
- **Input**: Optional connection name + required target group
- **Behavior**:
  - If connection name given: find by name, move to group
  - If no connection name: use currently selected connection
  - Auto-create target group if it doesn't exist
  - Register with undo system as UndoOpEdit
- **Success**: Flash "Moved {conn} to {group}"
- **Error**: Flash "Connection not found" or "No connection selected"
- **Undo**: Reverts connection's group field to previous value

## Leader Menu Contract (post-cleanup)

### Connections (c)
| Key | Label | Action |
|-----|-------|--------|
| c | Connect | connect-selected |
| q | Quick Connect | quick-connect |
| a | Add | add-connection |
| e | Edit | edit-connection |
| d | Delete | delete-connection |

### Find (f)
| Key | Label | Action |
|-----|-------|--------|
| f | Fuzzy Find | fuzzy-find |
| s | Sessions | find-sessions |
| t | By Tag | find-by-tag |
| g | By Group | find-by-group |
| r | Recent | show-recent |
| q | Frequent | show-frequent |
| v | Toggle Fav | toggle-favorite |
| a | All | show-all |
| b | Favorites | show-favorites |

### Sessions (s)
| Key | Label | Action |
|-----|-------|--------|
| l | List | list-sessions |
| k | Kill | kill-session (works directly) |
| a | Kill All | kill-all-sessions |

### Groups (g)
| Key | Label | Action |
|-----|-------|--------|
| c | Create | create-group (prompts for name) |
| d | Delete | delete-group (prompts for name) |
| m | Move | move-to-group (prompts for group) |
| f | Filter | filter-by-group (uses `:group` command) |

### Import (i)
| Key | Label | Action |
|-----|-------|--------|
| s | SSH Config | import-ssh |
| c | CSV | import-csv |
| j | JSON | import-json |

### Export (e)
| Key | Label | Action |
|-----|-------|--------|
| a | All | export-all |
| s | Selection | export-selection |
| g | Group | export-group |

### Tags (t)
| Key | Label | Action |
|-----|-------|--------|
| f | Filter | filter-by-tag |
| a | Add | add-tag |
| r | Remove | remove-tag |

### View (v)
| Key | Label | Action |
|-----|-------|--------|
| t | Table | view-table |
| d | Detail | view-detail |
| w | Wide | view-wide |
| e | Event Log | view-log |

### Sort (x)
| Key | Label | Action |
|-----|-------|--------|
| n | Name | sort-name |
| h | Host | sort-host |
| g | Group | sort-group |
| p | Protocol | sort-protocol |
| s | Status | sort-status |
| l | Latency | sort-latency |
| f | Favorite | sort-fav |

### Health (h)
| Key | Label | Action |
|-----|-------|--------|
| a | Check All | check-all |
| p | Pulse | pulse-view |

### Password (p)
| Key | Label | Action |
|-----|-------|--------|
| s | Show | show-password |
| c | Copy | copy-password |

### Data (d) — NEW
| Key | Label | Action |
|-----|-------|--------|
| n | Note | set-note (prompts for text) |
| f | Fields | manage-fields |

### Options (o)
| Key | Label | Action |
|-----|-------|--------|
| k | Keybindings | keybindings |

### Help (?)
Direct action — shows help overlay.
