# clab-tui

[![tests](https://github.com/tianyuxue/clab-tui/actions/workflows/test.yml/badge.svg?branch=main)](https://github.com/tianyuxue/clab-tui/actions/workflows/test.yml)
[![Integration](https://github.com/tianyuxue/clab-tui/actions/workflows/integration.yml/badge.svg?branch=main)](https://github.com/tianyuxue/clab-tui/actions/workflows/integration.yml)
[![Release](https://github.com/tianyuxue/clab-tui/actions/workflows/release.yml/badge.svg)](https://github.com/tianyuxue/clab-tui/actions/workflows/release.yml)
![Go](https://img.shields.io/badge/go-1.25-00ADD8)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

> 一个面向 [containerlab](https://containerlab.dev) 的终端 UI —— 无需离开键盘即可浏览、
> 部署和操作基于容器的网络 Lab。

[English](README.md) | **简体中文**

<!-- TODO 截图：拓扑标签页的主视图
![clab-tui topology view](docs/assets/screenshots/topology.png)
-->

## 功能特性

- **Lab 生命周期**：发现 `.clab.yml` 文件，然后进行部署、销毁和重新部署。
- **拓扑树**：按角色分组的设备树（spine/leaf/border/fw/...），展示实时的节点和链路
  状态、接口，以及详情面板。
- **节点控制**：启动、停止、重启、暂停和恢复节点。
- **会话**：在 TUI 内嵌终端会话（`docker exec`），支持插入/普通模式。
- **日志与统计**：实时的节点日志、CPU/内存 I/O 统计，以及各接口的计数器。
- **Netem**：对接口应用和清除延迟、抖动、丢包、速率和损坏。
- **抓包与追踪**：基于 eBPF 的入向/出向抓包，支持完整的 libpcap 过滤器，并可对
  Lab 进行实时路径追踪。
- **图视图**：在浏览器中打开分层拓扑图。
- **键盘驱动**：vim 风格的导航，以及 `space` which-key 菜单。

## 环境要求

clab-tui **不** 内置 containerlab。请先安装以下内容：

- **Linux**（使用 Linux 命名空间、eBPF 和 Docker CLI）。
- 在 `PATH` 中可用的 **[containerlab](https://containerlab.dev/install/)**，命令名为 `containerlab`。
- **Docker**，且守护进程正在运行。

部分功能需要提升权限；参见 [权限](docs/permissions.md)。
从源码构建还需要 **Go 1.25** 以及下文列出的 C 库。

## 安装

### 预编译二进制（linux/amd64）

从 [Releases](https://github.com/tianyuxue/clab-tui/releases) 页面下载最新的归档包，
校验后将二进制文件放入 `PATH`：

```bash
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/latest/download/clab-tui_linux_amd64.tar.gz"
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/latest/download/checksums.txt"
sha256sum -c checksums.txt
tar -xzf "clab-tui_linux_amd64.tar.gz"
sudo install -m 0755 "clab-tui_linux_amd64/clab-tui" /usr/local/bin/clab-tui
```

### 从源码构建

静态构建（推荐；静态链接 libc、libpcap 和 libcap）：

```bash
sudo apt-get install -y libpcap-dev libcap-dev libsystemd-dev \
  libdbus-1-dev libibverbs-dev libnl-3-dev libnl-route-3-dev pkg-config
make build
```

如果无法获取静态依赖，可使用动态回退方案：

```bash
make build-dynamic
```

## 快速开始

```bash
# 扫描目录中的 Lab（默认为当前目录），并在 UI 中选择一个
clab-tui --dir /path/to/labs

# 直接打开单个 Lab
clab-tui --file /path/to/lab.clab.yml
```

典型流程：选择一个 Lab → `l` → Deploy（部署） → 查看拓扑树 → 按 `o` 打开
节点操作（shell 会话、启动/停止/重启、日志）→ 按 `space` 打开
which-key 菜单。参见 [快速开始](docs/getting-started.md)。

## 权限

clab-tui 在浏览和基于 Docker 的操作中无需 root 即可使用。当条件不满足时，特权操作会
显示 toast 提示且不会执行 —— 它绝不会静默失败。完整模型以及如何授予各项能力参见
[权限](docs/permissions.md)。

## 文档

- [文档索引](docs/index.md)
- [快速开始](docs/getting-started.md)
- [快捷键](docs/keybindings.md)
- [权限](docs/permissions.md)
- [常见问题](docs/troubleshooting.md)

## 快捷键

| 按键 | 操作 |
|-----|--------|
| `tab` / `shift+tab` | 下一个 / 上一个标签页 |
| `1`–`4` | 跳转到 Topology / Sessions / Node Logs / Ops Log |
| `space` | 打开 which-key 菜单 |
| `l` | Lab 操作（Topology） |
| `o` | 节点 / 接口操作（Topology） |
| `/` | 搜索（Topology） |
| `t` / `f` | 切换追踪 / 更改追踪过滤器（Topology） |
| `?` | 切换展开的帮助栏 |
| `q` | 退出 |

完整参考：[快捷键](docs/keybindings.md)。

## 项目状态

clab-tui 正在积极开发中。接口和快捷键可能会在次版本之间发生变化。参见
[CHANGELOG.md](CHANGELOG.md) 和 [Releases](https://github.com/tianyuxue/clab-tui/releases) 页面。

## 贡献

欢迎提交 issue 和 pull request。请先阅读 issue 和 pull request 模板，并在提交 PR 前
运行 `go test ./...` 和 `go vet ./...`。

## 许可证

[Apache-2.0](LICENSE)。

## 致谢

- [containerlab](https://containerlab.dev) —— 本 UI 所驱动的 Lab 编排工具。
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) 和
  [Lip Gloss](https://github.com/charmbracelet/lipgloss) —— TUI 技术栈。
- [vscode-containerlab](https://github.com/srl-labs/vscode-containerlab) —— 键盘优先的
  containerlab 工作流灵感来源。
