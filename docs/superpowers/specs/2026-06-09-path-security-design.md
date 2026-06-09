# Path Security Design

## Goal

Prevent user-controlled iroll names and relative paths from reading, writing, or deleting files outside their intended roots.

## Rules

- An iroll name is one directory name. Reject empty names, `.`, `..`, absolute paths, and names containing `/` or `\`.
- A resource path may contain subdirectories, but it must be relative and remain inside its declared root.
- Unsafe input fails closed with an explicit error. Paths are never silently cleaned into a different accepted path.
- SQL context references are outside this change.

## Architecture

Add a small `safepath` package with two public operations:

- `ValidateName(name string) error`
- `Join(root, relativePath string) (string, error)`

Use these operations at every filesystem boundary:

- iroll store name resolution and ZIP extraction
- Layerfile `FROM`, `MIGRATE`, and `COPY`
- context `@file` resolution

## Testing

Tests cover valid names and nested relative paths, invalid names, absolute paths, traversal paths, ZIP Slip archives, Layerfile traversal, and context file traversal.

