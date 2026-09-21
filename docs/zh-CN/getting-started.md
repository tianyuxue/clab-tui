# 快速开始

[English](../getting-started.md) | **简体中文**

## 环境要求

clab-tui 不捆绑 containerlab。请先安装以下内容：

- **Linux**。
- `PATH` 中的 **[containerlab](https://containerlab.dev/install/)**。
- 已运行守护进程的 **Docker**。
- 如需从源码构建：**Go 1.25**。

## 安装

### 方式 A：预编译二进制文件（linux/amd64）

从 [Releases](https://github.com/tianyuxue/clab-tui/releases) 页面下载，校验 checksum
后安装：

```bash
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/latest/download/clab-tui_linux_amd64.tar.gz"
curl -fsSLO "https://github.com/tianyuxue/clab-tui/releases/latest/download/checksums.txt"
sha256sum -c checksums.txt
tar -xzf "clab-tui_linux_amd64.tar.gz"
sudo install -m 0755 "clab-tui_linux_amd64/clab-tui" /usr/local/bin/clab-tui
```

### 方式 B：从源码构建

默认构建是静态的，会以静态方式链接 libc、libpcap 和 libcap。在 Debian/Ubuntu 上请先安装
开发包：

```bash
sudo apt-get install -y libpcap-dev libcap-dev libsystemd-dev \
  libdbus-1-dev libibverbs-dev libnl-3-dev libnl-route-3-dev pkg-config
make build
```

如果静态库不可用，可使用动态回退方案：

```bash
make build-dynamic
```

<!-- TODO 截图：静态构建输出
![构建 clab-tui](../assets/screenshots/getting-started-build.png)
-->

## 配置

clab-tui 会读取 `$XDG_CONFIG_HOME/clab-tui/config.yaml`，若不存在则回退到
`~/.config/clab-tui/config.yaml`。文件缺失时使用默认值。

```yaml
containerlab:
  bin_path: ""    # containerlab 二进制文件路径；留空 = 从 PATH 解析
  timeout: 300    # 保留
theme: default     # 保留
```

目前仅 `containerlab.bin_path` 会生效。

## 运行

```bash
# 扫描目录中的 .clab.yml / .clab.yaml 文件（默认为当前目录）
clab-tui --dir /path/to/labs

# 直接打开单个 Lab，跳过启动时的选择器
clab-tui --file /path/to/lab.clab.yml
```

`--dir` 与 `--file` 互斥。启动时，clab-tui 会把找到的 Lab 文件数量打印到 stderr。

<!-- TODO 截图：Lab 选择器
![Lab 选择器](../assets/screenshots/getting-started-lab-picker.png)
-->

## 基本流程

1. **选择 Lab。** 使用 `--dir` 时，启动时会打开 Lab 选择器。选择一个 Lab 并按
   `Enter`。随时可用 `l` → Switch Lab（切换 Lab）来更换 Lab。
2. **部署。** 按 `l` → Deploy（部署）。输出会流式写入 Ops Log 标签页。此操作需要
   containerlab 权限（参见[权限](permissions.md)）。
3. **浏览拓扑。** 拓扑标签页左侧是按角色分组的设备树，右侧是详情面板。用 `j`/`k`
   移动，按 `Enter` 展开某个节点的连接，按 `x` 切换所有连接，按 `i` 切换接口行。

   <!-- TODO 截图：带详情面板的拓扑树
   ![拓扑树](../assets/screenshots/getting-started-topology.png)
   -->
4. **对节点和接口进行操作。** 按 `o` 打开操作菜单。对节点：SSH/shell、Start（启动）、
   Stop（停止）、Restart（重启）、Pause（暂停）、Unpause（恢复）、View Logs（查看日志）。
   对接口：Packet Capture（抓包）、Set Netem（设置 Netem）、Clear Netem（清除 Netem）。

   <!-- TODO 截图：节点操作菜单
   ![节点操作](../assets/screenshots/getting-started-node-actions.png)
   -->
5. **打开会话。** 选择 SSH 会打开一个内嵌的 shell 会话。按 `ctrl+\` 进入普通模式，
   按 `s` 切换会话，在普通模式下按 `q` 关闭。

   <!-- TODO 截图：内嵌会话
   ![会话](../assets/screenshots/getting-started-session.png)
   -->
6. **抓包。** 在某个接口上选择 Packet Capture（抓包），挑选一个过滤器预设
   （BGP、OSPF、ICMP、ARP、All 或 Custom），然后在抓包面板中按 `w` 保存 pcap。
   需要 `CAP_BPF` 级别的权限。

   <!-- TODO 截图：抓包面板
   ![抓包面板](../assets/screenshots/capture.png)
   -->
7. **销毁。** 完成后按 `l` → Destroy（销毁）。

## 后续步骤

- [快捷键](keybindings.md)
- [权限](permissions.md)
- [常见问题](troubleshooting.md)
