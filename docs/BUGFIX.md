# Bug Fix Log

## 2026-03-28: StopAll 导致共享 SSH 连接的隧道无法全部停止

### 现象

当多个隧道共用同一个 SSH 连接时，点击"全部停止"后，部分隧道显示已停止，但另一个隧道无法正常停止，UI 反复尝试 Stop 却不生效。

### 复现步骤

1. 配置两个隧道（如 PGSQL:5432、Redis:6379）使用同一个 SSH 服务器
2. 点击"全部启动" → 两个隧道均 Connected，共享一个 SSH Client（refCount=2）
3. 点击"全部停止" → 第一个隧道正常停止，第二个隧道因 SSH 连接被提前关闭而进入异常状态

### 根本原因

**`WaitForDisconnectContext`（client.go）在 context 取消时强制关闭 SSH 连接，破坏了其他隧道正在使用的共享连接。**

时序分析：

```
StopAll()
  ├─ StopTunnel(Redis)          ← 第一个隧道
  │   ├─ cancel(ctx)
  │   ├─ listener.Close()
  │   └─ wg.Wait()
  │       └─ SSH monitor goroutine:
  │           └─ WaitForDisconnectContext(ctx)
  │               └─ ctx.Done() → c.closeConn(client)  ← BUG: 强制关闭共享的 SSH 连接!
  │
  ├─ ReleaseClient(refCount: 2→1)  ← Redis 释放，但 PGSQL 还在用
  │
  └─ StopTunnel(PGSQL)         ← 第二个隧道
      └─ PGSQL 的 SSH monitor 已检测到连接断开
          → 进入 reconnect 或异常状态
```

关键代码（修复前）：

```go
// client.go WaitForDisconnectContext
case <-ctx.Done():
    c.closeConn(client)  // ← 无条件关闭，不管其他隧道是否还在用
```

`closeConn` 直接调用 `ssh.Client.Close()`，而此时 refCount 仍为 1（另一个隧道还在使用），导致另一个隧道的 SSH 连接被意外断开。

### 修复方案

**移除 `WaitForDisconnectContext` 中 context 取消时的强制关闭逻辑。**

当 context 被取消时（用户停止隧道），直接返回，不关闭 SSH 连接。连接的生命周期完全由 `ClientManager.ReleaseClient` 的引用计数管理——当最后一个隧道释放时，refCount 降至 0，才会调用 `Disconnect()` 关闭连接。

修复后的时序：

```
StopAll()
  ├─ StopTunnel(Redis)
  │   ├─ cancel(ctx)
  │   ├─ listener.Close()
  │   └─ wg.Wait()
  │       └─ SSH monitor goroutine:
  │           └─ WaitForDisconnectContext(ctx)
  │               └─ ctx.Done() → 直接返回，不关闭连接  ← FIX
  │
  ├─ ReleaseClient(refCount: 2→1)  ← Redis 释放，连接保留给 PGSQL
  │
  └─ StopTunnel(PGSQL)
      ├─ cancel(ctx)
      ├─ listener.Close()
      └─ wg.Wait() → 正常退出
          └─ ReleaseClient(refCount: 1→0)
              └─ Disconnect()  ← 最后一个用户，安全关闭连接
```

### 涉及文件

| 文件 | 改动 |
|------|------|
| `internal/ssh/client.go` | `WaitForDisconnectContext`: 移除 `c.closeConn(client)` 调用 |
| `internal/ssh/tunnel.go` | `acceptConnectionsOrWaitDisconnect`: SSH monitor 增加 debug 日志 |
| `internal/ssh/tunnel.go` | `StopTunnel`: 增加状态和 goroutine 完成日志 |
| `internal/ssh/tunnel.go` | `StopAll`: 增加进度追踪日志 |

---

## 2026-03-28: WaitForDisconnectContext goroutine 泄漏

### 现象

修复上一个 bug 后，每次调用 `WaitForDisconnectContext` 且 context 被取消时，内部启动的 `client.Wait()` goroutine 会泄漏，直到 SSH 连接自然断开或应用关闭。

### 根本原因

```go
func (c *Client) WaitForDisconnectContext(ctx context.Context) {
    done := make(chan struct{})
    go func() {
        client.Wait()   // ← 阻塞，直到 SSH 连接关闭
        close(done)
    }()

    select {
    case <-done:
        // SSH 断开，goroutine 已退出
    case <-ctx.Done():
        return           // ← 直接返回，但上面的 goroutine 还在跑 client.Wait()
    }
}
```

当 context 取消时，函数返回，但 `client.Wait()` goroutine 没有任何机制被回收。如果多个隧道共享一个 SSH 连接，部分隧道停止后，这些泄漏的 goroutine 会一直存在，直到：

- 最后一个隧道停止 → `ReleaseClient` 调用 `Disconnect()` → SSH 连接关闭 → `client.Wait()` 返回
- SSH 连接自然断开
- 应用关闭

### 修复方案

**将 `client.Wait()` goroutine 注册到 `Client.wg`（已有的 WaitGroup），由 `Disconnect()` 统一等待清理。**

```go
c.wg.Add(1)
go func() {
    defer c.wg.Done()   // ← 注册到 Client 的 WaitGroup
    client.Wait()
    close(done)
}()
```

`Disconnect()` 已有 `c.wg.Wait()` 调用，确保所有注册的 goroutine 在连接关闭后都能被等待回收，不会永久泄漏。

### 泄漏路径分析

| 场景 | goroutine 数量 | 清理时机 |
|------|--------------|---------|
| 停止单隧道（唯一使用者） | 1 | `ReleaseClient` refCount=0 → `Disconnect` → `wg.Wait` |
| 停止 N-1 个隧道（共 N 个共享连接） | N-1 | 最后一个隧道停止时统一清理 |
| 全部停止（StopAll） | N | `StopAll` 完成后，最后一个 `ReleaseClient` 触发清理 |

所有路径均为有界泄漏，最迟在应用关闭时清理完毕。

### 涉及文件

| 文件 | 改动 |
|------|------|
| `internal/ssh/client.go` | `WaitForDisconnectContext`: 将 `client.Wait()` goroutine 注册到 `c.wg` |
