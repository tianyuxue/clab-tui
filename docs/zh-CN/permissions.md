# 权限

[English](../permissions.md) | **简体中文**

clab-tui 的大部分浏览操作和基于 Docker 的操作都无需 root 即可使用。需要较高权限的功能
会预先检查；当要求未满足时，clab-tui 会给出提示，而**不会**执行该操作，避免静默失败。

## 权限分级

| 操作 | 要求 |
|------------|-------------|
| 浏览拓扑、打开图视图 | 无 |
| 会话、节点日志、统计、节点启动/停止/重启/暂停/恢复、netem、接口 IP | Docker 访问 |
| 部署、销毁、重新部署、实时事件流、`inspect interfaces` | containerlab 权限 |
| 抓包与路径追踪 | 追踪 capabilities |

## 如何检测 capabilities

启动时 clab-tui 会检查：

- **root**——有效 UID 为 0。
- **Docker 组**——你的用户属于 `docker` 组。
- **containerlab SUID**——`containerlab` 二进制文件带有 setuid 位。
- **Linux capabilities**——`CAP_BPF`、`CAP_NET_ADMIN`、`CAP_SYS_ADMIN`、`CAP_SYS_PTRACE`。
- **BTF**——存在 `/sys/kernel/btf/vmlinux`（eBPF 所需）。

## 以 root 运行

`clab-tui --sudo ...` 适用于任意安装位置：它会用二进制的绝对路径通过 sudo 重新执行，
因此安装在 `~/.local/bin` 时不会被 sudo 的 `secure_path` 挡住。如果用
`make install-system` 安装到 `/usr/local/bin`，也可以直接运行 `sudo clab-tui ...`。

重新执行后的进程以 root 身份运行，因此读取的是 root 的配置
（`/root/.config/clab-tui/config.yaml`）而不是你的用户配置，它创建的文件也归 root
所有。

## 授予访问权限

### Docker 访问

将你的用户加入 `docker` 组并重新登录：

```bash
sudo usermod -aG docker "$USER"
```

也可以使用 `clab-tui --sudo`（安装到系统目录后可用 `sudo clab-tui`）运行。

### 部署 / 销毁 / 事件

除非二进制文件设置了 setuid，否则 containerlab 需要 root。可以用 `clab-tui --sudo`
运行，或启用 SUID：

```bash
sudo chmod u+s "$(command -v containerlab)"
```

### 抓包与追踪

追踪需要满足以下条件之一：

- 使用 `clab-tui --sudo`（或 `sudo clab-tui`）运行；或
- 授予上述四个 capabilities，并确保内核 BTF 可用。

如果要求未满足，启动追踪或抓包时会显示：

> packet tracing needs root (run with sudo) or CAP_BPF+CAP_NET_ADMIN+CAP_SYS_ADMIN+CAP_SYS_PTRACE

## 受限模式

当 containerlab 权限不可用时，状态栏会显示 `limited mode`，实时接口状态可能无法获取。
没有实时数据的链路会显示为 `UNKNOWN` 而不是 `DOWN`，以免将过期或缺失的读数误判为链路
故障。

## 注意事项

- 使用 `sudo` 运行会改变配置文件和生成文件的属主用户，并且 clab-tui 会读取
  `/root/.config/clab-tui/` 下 root 的配置。
- 如果 `containerlab` 安装在 root 的 `PATH` 之外（例如 `~/.local/bin`），请在 root 的
  配置中设置 `containerlab.bin_path`，或将其安装到系统目录。
- 为 `containerlab` 设置 SUID 会让任何能执行它的人获得特权；请仅在你信任的机器上这样做。
- 在宿主机上，加入 Docker 组实际上等同于拥有 root 权限。
