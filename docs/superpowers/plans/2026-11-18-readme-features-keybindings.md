# README 功能与快捷键更新实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 更新中英文根目录 README，使功能介绍和高频快捷键与当前 clab-tui 实现一致。

**Architecture:** 仅修改 `README.md` 与 `README.zh-CN.md`。保持 README 简明，完整的
菜单层级、表单和浮层说明继续由 `docs/keybindings.md` 承担。

**Tech Stack:** Markdown；现有 clab-tui Go/TUI 实现作为事实来源。

**Spec:** `docs/superpowers/specs/2026-11-18-readme-features-keybindings-design.md`

## Global Constraints

- 不修改程序代码、快捷键行为或完整快捷键文档。
- 中英文 README 的章节结构和信息范围保持一致。
- 仅突出高频入口，复杂操作链接到现有快捷键文档。

## Review Focus

- 四个标签页名称与数字快捷键必须一致：Topology、Sessions、Node Logs、Ops Log。
- Sessions 的插入模式不能误导用户：`ctrl+\` 或快速双击 `Esc` 才进入普通模式。
- `space` 菜单是模态 which-key 菜单，菜单中可用操作随当前标签页变化。
- 抓包面板的 `q/c/p/w` 仅在抓包面板可见时生效，不能描述成全局快捷键。
- README 中的文档链接和中英文对应链接必须指向现有文件。

### Task 1: 同步更新中英文 README

**Files:**
- Modify: `README.md`
- Modify: `README.zh-CN.md`

**Interfaces:**
- Consumes: 当前 `internal/tui/model.go`、`internal/tui/keys.go`、
  `internal/tui/tabs/whichkey.go`、`internal/tui/tabs/sessions.go` 以及
  `docs/keybindings.md` 的行为说明。
- Produces: 面向新用户的功能概览和高频快捷键速查表。

- [ ] **Step 1: 扩充 Features/功能特性**

在现有条目基础上明确四个标签页、键盘导航、which-key 菜单、自动刷新和
会话模式；保留现有 Lab 生命周期、拓扑、节点控制、日志统计、Netem、
抓包追踪和图视图说明。

- [ ] **Step 2: 重写 README 快捷键速查表**

覆盖全局导航（`q`、`?`、`space`、标签页切换和 `1`–`4`）、拓扑高频操作
（`j/k`、`g/G`、`Enter`、`x`、`i`、`/`、`n/N`、`t`、`f`、`l`、`o`）、
日志操作（`f/c/g/G`）、抓包面板（`q/c/p/w`）以及 Sessions 的模式切换。
明确上下文范围，并保留完整参考文档链接。

- [ ] **Step 3: 同步英文与中文表达**

确保两份 README 的功能顺序、快捷键范围和注意事项一致，仅翻译语言，不引入
一方独有的行为描述。

- [ ] **Step 4: 检查文档链接与内容一致性**

运行路径检查，确认 `docs/getting-started.md`、`docs/keybindings.md`、
`docs/permissions.md`、`docs/index.md` 及中文对应文档存在；使用搜索核对
README 中的标签页、快捷键和 Sessions 模式描述没有遗漏或冲突。

- [ ] **Step 5: 查看最终 diff**

运行：

```bash
git diff --check
git diff -- README.md README.zh-CN.md
```

预期：无空白错误；diff 仅包含 README 文档改动且中英文结构对应。
