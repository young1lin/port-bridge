# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.0.2] - 2026-03-28

### Fixed

- StopAll 共享 SSH 连接时 force-close 导致部分隧道无法停止（`WaitForDisconnectContext` 在 context 取消时不再强制关闭连接，连接生命周期由引用计数管理）
- `WaitForDisconnectContext` goroutine 泄漏（将 `client.Wait()` goroutine 注册到 `Client.wg`，由 `Disconnect()` 统一回收）

### Changed

- 重命名项目：forward-port → port-bridge（文档、构建输出、配置路径）
- Go 版本要求更新至 1.26
- Makefile 测试目标拆分：`test-unit` / `test-gui` / `test-all`
- 文档同步更新：CLAUDE.md、AGENTS.md、README.md、README_EN.md

### Added

- Spinner 动画资源（30 帧 SVG）
- i18n 翻译改进
- 更新器缓存机制与重试逻辑
- Storage 测试覆盖率提升（密钥环集成）
- `docs/BUGFIX.md` bug 修复记录

### Removed

- `scripts/generate_patches.sh`（已废弃）

## [0.0.1] - 2026-03-25

### Added

- SSH 端口转发 GUI 工具初始版本
- 基于 Fyne v2 的跨平台桌面界面
- SSH 连接管理（密码/密钥认证、SOCKS5 代理）
- 隧道生命周期管理（启动/停止/自动重连）
- JSON 配置持久化
- 日志自动轮转
- 系统托盘集成
