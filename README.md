# PortBridge

[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go)](https://go.dev/)
[![CI](https://github.com/young1lin/port-bridge/actions/workflows/ci.yml/badge.svg)](https://github.com/young1lin/port-bridge/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/young1lin/port-bridge/branch/main/graph/badge.svg)](https://codecov.io/gh/young1lin/port-bridge)
[![Go Report Card](https://goreportcard.com/badge/github.com/young1lin/port-bridge)](https://goreportcard.com/report/github.com/young1lin/port-bridge)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/young1lin/port-bridge)](https://github.com/young1lin/port-bridge/releases)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)](https://github.com/young1lin/port-bridge/releases)
[![Downloads](https://img.shields.io/github/downloads/young1lin/port-bridge/total)](https://github.com/young1lin/port-bridge/releases)

[English](README_EN.md) | 简体中文

PortBridge 是一个基于 Go 和 Fyne 的桌面应用，用来管理 SSH 连接、端口转发隧道和自动重连。它面向日常开发、远程访问和本地代理场景，提供可视化的连接管理体验。

## 功能特性

- 保存和管理 SSH 连接配置，支持密码和密钥认证
- 创建本地端口转发隧道，支持运行状态和错误提示
- SSH 连接复用、断线检测与自动重连
- 系统托盘、日志轮转、自动更新
- 内置中英文界面切换

## 项目结构

- `cmd/port-bridge`: 应用入口、托盘、平台相关启动逻辑、内置翻译
- `internal/app`: 应用服务编排
- `internal/ssh`: SSH client、隧道生命周期、known_hosts
- `internal/storage`: 配置持久化和密钥环集成
- `internal/ui`, `internal/presenter`: Fyne 界面和交互逻辑
- `internal/updater`, `internal/logger`, `internal/i18n`: 更新、日志、多语言支持

## 平台说明

### Windows

直接下载 `.exe` 运行，无需额外配置。

### Linux（Ubuntu Desktop 等带桌面环境）

需要系统已安装 OpenGL 和 X11/Wayland 依赖，Ubuntu Desktop 默认已具备，直接运行即可。若报 OpenGL 相关错误，执行：

```bash
sudo apt install libgl1-mesa-dev xorg-dev
```

### macOS

macOS 对未签名的第三方 GUI 应用有 Gatekeeper 限制。下载后若提示 **"无法打开，因为 Apple 无法检查是否包含恶意软件"** 或 **"已损坏"**，在终端执行以下命令解除隔离后即可运行：

```bash
xattr -cr /path/to/PortBridge.app
```

或在访达中右键 → 打开 → 仍要打开。

> 原因：正式签名和公证需要苹果开发者账号（$99/年），当前版本为未签名构建。

## 开发

```bash
make deps
make run
make test-unit
make test-gui
make test-all
make test-coverage
```

- `make build`: 生成发布版 `build/port-bridge.exe`
- `make build-debug`: 生成带控制台的调试版本
- `make test-unit`: 纯单元测试，不依赖 Fyne / CGO / GUI 运行时
- `make test-gui`: GUI 测试，依赖 Fyne 和 CGO 工具链
- `make test-all`: 单元测试 + GUI 测试 + SSH 集成测试
- `make test-coverage`: 核心包覆盖率门槛为 95%+

项目使用 Go `1.26`，module path 为 `github.com/young1lin/port-bridge`。

## 发布

推送形如 `v1.2.3` 的 tag 会触发 GitHub Release 工作流。预编译版本可在 [Releases](https://github.com/young1lin/port-bridge/releases) 页面下载。

## 许可证

[MIT](LICENSE)
