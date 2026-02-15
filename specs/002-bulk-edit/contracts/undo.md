# Undo System Contract: Batch Operations

## New Constant

```
UndoOpBatch OpType = 4
```

## Modified Struct

```
Operation {
    ...existing fields...
    Children []Operation  // NEW: child operations for batch (nil for non-batch)
}
```

## New Method

```
PushBatch(name string, children []Operation)
  - Creates Operation{Type: UndoOpBatch, Name: name, Children: children}
  - Pushes to undo stack as single entry
  - Clears redo stack
```

## Undo Behavior (UndoOpBatch)

```
performUndo for UndoOpBatch:
  for each child in REVERSE order:
    apply child's undo logic (same as existing single-item cases)
  flash: "Undo: <batch.Name> (N items)"
```

## Redo Behavior (UndoOpBatch)

```
performRedo for UndoOpBatch:
  for each child in FORWARD order:
    apply child's redo logic (same as existing single-item cases)
  flash: "Redo: <batch.Name> (N items)"
```

## Bulk Move Contract (visual mode)

```
:mv <group> with visual mode active:
  1. ids = ResolveVisualIDs(visualState, table.rows)
  2. if len(ids) == 0 → error flash
  3. snapshots_before = snapshot each connection
  4. Auto-create target group if missing
  5. moved, total = ExecuteBulkMove(cfg, ids, group)
  6. snapshots_after = snapshot each connection
  7. Build children: Operation{Type: UndoOpEdit, Before: before, After: after} for each
  8. PushBatch("bulk move to <group>", children)
  9. Exit visual mode
  10. Flash "Moved X of Y to <group>"
```

## Retrofit Contract

### tagAddVisual / tagRemoveVisual
```
Replace: N individual Push(Operation{...}) calls
With: Collect operations into []Operation, then PushBatch("bulk tag add <tag>", ops)
```

### delete-visual
```
Replace: N individual Push(Operation{...}) calls
With: Collect operations into []Operation, then PushBatch("bulk delete", ops)
```
