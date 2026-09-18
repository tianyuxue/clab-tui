# 常见问题

[English](../troubleshooting.md) | **简体中文**

## clab-tui 报告找不到 containerlab

除非在配置文件中设置了 `containerlab.bin_path`，否则 `clab-tui` 会从 `PATH` 解析
`containerlab`。请安装 containerlab，或显式设置路径。

## 部署、销毁或追踪时显示权限提示

当要求未满足时，这是预期行为。请参阅[权限](permissions.md)，了解如何授予 Docker 访问、
containerlab SUID 或追踪 capabilities。

## 状态栏显示 "limited mode"

containerlab 权限不可用，因此无法读取实时接口状态。浏览功能仍可正常使用。要获取完整
数据，请使用 `sudo` 运行，或启用 containerlab SUID。

## 链路显示 UNKNOWN

`UNKNOWN` 表示实时接口数据不可用（通常是受限模式的情况），而不是链路故障。请参阅
[权限](permissions.md)。

## Docker 权限被拒绝

将你的用户加入 `docker` 组并重新登录，或使用 `sudo` 运行：

```bash
sudo usermod -aG docker "$USER"
```

## 静态构建因缺少 pkg-config 库而失败

静态构建需要开发包。在 Debian/Ubuntu 上：

```bash
sudo apt-get install -y libpcap-dev libcap-dev libsystemd-dev \
  libdbus-1-dev libibverbs-dev libnl-3-dev libnl-route-3-dev pkg-config
```

如果静态库不可用，也可使用 `make build-dynamic`。

## 图视图无法打开浏览器

clab-tui 会以 toast 形式打印图 URL，并使用 `xdg-open` 打开它。在无头或远程环境中浏览器
无法打开；请手动访问打印出的 `http://localhost:<port>` URL。端口默认为 `50080`，可通过
`CLAB_TUI_GRAPH_PORT` 更改。

如果端口已被占用，clab-tui 会报告图错误；请停止冲突进程，或设置一个不同的
`CLAB_TUI_GRAPH_PORT`。

## 配置文件被忽略

配置文件位于 `$XDG_CONFIG_HOME/clab-tui/config.yaml`，当 `XDG_CONFIG_HOME` 未设置时为
`~/.config/clab-tui/config.yaml`。解析错误会打印到 stderr，并使用默认值。目前只有
`containerlab.bin_path` 会生效；`timeout` 和 `theme` 为保留项。

## Edit YAML 打开了错误的编辑器

编辑器取自 `$EDITOR`（默认为 `vi`）。请将 `EDITOR` 设置为你偏好的编辑器。
