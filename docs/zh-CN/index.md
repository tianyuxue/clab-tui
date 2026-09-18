# clab-tui 文档

[English](../index.md) | **简体中文**

clab-tui 是一款面向 [containerlab](https://containerlab.dev) 的终端 UI。它会扫描你的
`.clab.yml` 文件，让你能够浏览拓扑、运行 Lab（实验环境）生命周期、打开节点会话、
查看日志与统计、施加链路损伤，以及抓包或追踪流量——全部通过键盘完成。

## 适用人群

需要运行 containerlab 实验环境，并且偏好终端原生工作流而非图形界面的网络工程师和开发者。

## 环境要求一览

- Linux
- `PATH` 中的 [containerlab](https://containerlab.dev/install/)（不随本程序捆绑）
- 已运行守护进程的 Docker

## 文档

| 页面 | 内容 |
|------|------|
| [快速开始](getting-started.md) | 安装、配置并运行你的第一个 Lab |
| [快捷键](keybindings.md) | 完整的键盘参考 |
| [权限](permissions.md) | 哪些功能无需 root，以及如何授予 capabilities |
| [常见问题](troubleshooting.md) | 常见错误与修复方法 |

<!-- TODO 截图：文档首页主视觉
![clab-tui 拓扑视图](../assets/screenshots/topology.png)
-->
