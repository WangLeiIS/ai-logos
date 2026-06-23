# Layerfile → Irollfile 重命名

> **Goal:** 将构建文件名从 Layerfile 改为 Irollfile，类比 Dockerfile。

**Date:** 2026-06-23

## 动机

`docker build` 默认读当前目录的 `Dockerfile`。Logos 的构建文件应命名为 `Irollfile`（而非 `Layerfile`），保持一致的直觉。默认路径和 Go 类型名同步重命名。

## 变更

### Go 代码

| 文件 | 变更 |
|------|------|
| `builder/layerfile.go` → `builder/irollfile.go` | `Layerfile`→`Irollfile`，`ParseLayerfile`→`ParseIrollfile` |
| `builder/build.go` | `*Layerfile`→`*Irollfile` |
| `builder/build_test.go` | 所有引用 |
| `cmd/build.go` | 默认值 `"Layerfile"`→`"Irollfile"`，帮助文本 |
| `e2e/testenv/setup.go` | 路径、方法名 `LayerfilePath`→`IrollfilePath` |
| `e2e/scenario_lifecycle_test.go` | 临时文件名 |
| `e2e/scenario_edge_test.go` | 临时文件名 |
| `cmd/loop_integration_test.go` | 引用路径 |

### 示例文件

- `examples/layer2/Layerfile` → `examples/layer2/Irollfile`

### 文档

- `CLAUDE.md`、`README.md`、`docs/blueprint.md`
- `docs/iroll-layered-build-spec.md`、`docs/iroll-protocol-v1.md`、`docs/rebot-roll.md`
- `skills/logos-1/skill.md`

### 不做

- `FROM`/`MIGRATE`/`COPY` 指令名不变
- `docs/superpowers/plans/` 历史文档不变
