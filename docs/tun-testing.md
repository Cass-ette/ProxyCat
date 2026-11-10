# TUN 切换验证

## 保持开发会话联网

修改源码、运行回归测试及构建安装包时，不安装或启动开发版本，不对运行中的 ProxyCat 执行 TUN、系统代理或内核切换。不能把当前会话中的 `finally` 或“稍后切回来”当作断网保护：断网后，开发会话可能无法继续执行。

TUN 启动和失败恢复测试使用临时目录、`sleep` 子进程、Unix 套接字及随机本地 TCP 端口。CLI 测试在 `TestMain` 中替换 `pgrep`、`networksetup`、`scutil`，使用临时 HOME，并阻止 HTTP 客户端连接实际的 7890 / 9090 端口。测试不申请管理员权限，不创建 TUN，不修改 DNS 或路由。

```bash
cd helper
go test -race ./internal/tun ./internal/core ./cmd/proxyctl
```

应用侧使用模拟 helper 验证 TUN 状态显示、失败提示、退出及校园网断开流程：

```bash
bash scripts/test-connection-recovery.sh
bash scripts/test-campus-network.sh
bash scripts/test-shutdown.sh
```

`bash scripts/package-unsigned.sh` 只构建分发文件；`bash scripts/test-dmg.sh dist/ProxyCat-版本号.dmg` 只读挂载并检查安装包，均不安装或启动应用。开发期间的修复并入尚未发布的版本，遵循 [版本号规则](versioning.md)，不为每次测试单独升号。

## 真实 TUN 验收

隔离测试只验证切换顺序和错误恢复，不能证明管理员授权、真实路由、DNS 或外网可达性。真实验收由本机用户在方便恢复网络时进行，保留旧安装包，不把正在依赖该代理的远程会话作为唯一操作入口。

1. 记录当前节点和路由模式，确认普通代理可以访问一个已知网站。
2. 首次使用时，取消一次系统助手安装授权，确认普通代理仍可用。
3. 开启 TUN，确认应用显示已开启，再用忽略系统代理的应用验证访问。
4. 关闭 TUN，确认普通代理恢复，节点和模式保留。
5. 开启 TUN 后退出 ProxyCat，确认系统代理关闭、TUN 内核退出，再检查原本可直连的网站。
6. 重新打开应用，再次开启 TUN，确认不再要求输入密码。只有首次安装、卸载后重装或需要升级系统助手时才申请授权。

新逻辑在停止普通内核前要求后台服务握手成功；启动失败后先确认 TUN 子进程退出，再尝试恢复普通内核。恢复成功表示控制接口、原模式及节点选择和本地监听端口可用，不表示某个海外节点或网站一定能连通。

若需要自动执行真实网络切换，应先准备独立于 Codex 会话运行的本地定时恢复任务，并验证恢复任务可用，再执行切换。当前隔离测试和构建流程不执行这类切换。

## 系统助手

每个授权用户有独立的 launchd 作业 `com.cassette.proxycat.tun.<UID>`。程序和固定内核保存在 `/Library/PrivilegedHelperTools/com.cassette.proxycat.tun.<UID>/`，作业文件在 `/Library/LaunchDaemons/`。这些文件由 root 所有，系统助手只通过该用户可访问的本地 Unix 套接字接收会话准备请求；不监听网络端口，不提供任意命令执行接口。

每次会话仍要确认拥有它的用户和应用进程。应用退出、取消启动或停止 TUN 后，会话及其内核结束；系统助手继续空闲，后续开关无需重新安装。系统助手日志为安装目录中的 `service.log`，内核日志仍在应用的日志文件夹中。

用户可在关闭 TUN 后通过 **更多 → 卸载 TUN 系统助手…** 移除，或运行：

```bash
/Applications/ProxyCat.app/Contents/Resources/proxyctl tun uninstall-helper
```

卸载需要管理员授权，移除 launchd 作业及其程序，不会关闭普通代理。第一次安装和安装失败回滚的自动化测试只使用临时文件及模拟 launchd 操作。
