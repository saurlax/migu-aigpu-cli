# 登录与会话管理

推荐使用浏览器自动导入：

```sh
migu auth login
# 自动检测不到浏览器时指定路径
migu auth login --browser "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
# 登录等待时间，默认10分钟，最多30分钟
migu auth login --wait 15m
```

CLI 打开独立的可见 Chrome/Edge 窗口；在里面正常完成短信验证码登录即可，不用开发者工具或粘贴脚本。CLI 自动检测必要会话，用一次只读实例查询验证，然后加密保存。取消、超时或验证失败保留原会话。验证成功后关闭专用浏览器，删除临时浏览器配置。进度写入 stderr，`--json` 的成功元数据写入 stdout，不输出凭证。

Windows Edge 已完成真实网页登录、自动验证、保存后再次查询和专用浏览器退出清理的完整实测。其他系统通过编译和单元测试，仍需当地桌面环境的真实登录验收。

需要本机已有 Chrome、Edge 或兼容 Chromium 浏览器及桌面环境；不会自动下载浏览器，不接管日常浏览器配置。用户关闭全部登录窗口会结束命令。登录检测仅在该命令运行期间进行，不启动后台任务。异常强制终止进程可能留下临时浏览器数据；不要将临时配置目录分享出去。

2026-10-05 已完成真实登录后的 OAuth 回调验证：现有网页客户端拒绝 `http://127.0.0.1:54321/callback`，报错 `Invalid redirect ... does not match one of the registered values`。自动登录使用官方网页登录后的浏览器会话导入，并非官方第三方 OAuth 接入。

也可以使用手动导入作为备用：

```sh
migu auth capture
```

1. 在浏览器登录 https://aigpu.migu.cn。
2. CLI 输出一个本地 `.js` 文件路径。打开文件，将其内容粘贴到**已登录的平台页面**的开发者工具 Console 执行。浏览器可能要求先确认允许粘贴，请阅读并检查脚本。
3. 浏览器将必要的登录态直接发送到随机端口的 `127.0.0.1` 接收器，CLI 加密保存。脚本和终端输出不包含凭证；不要将浏览器登录态粘贴到聊天或提交到 Git。

接收器仅接受该平台的精确 Origin、随机路径和一次有效导入；成功、取消或 5 分钟超时后退出，脚本随即删除。`--wait 10m` 可延长导入时间。浏览器阻止网页访问本机网络时需要允许本次访问。

Windows 使用当前用户 DPAPI 加密文件 `%APPDATA%\migu-aigpu-cli\session.dpapi`。macOS 使用系统 Keychain，Linux 使用 Secret Service（需要 D-Bus 和已解锁的密钥环）。密钥环失败不会降级到明文文件；无桌面 Linux 需要自行配置 Secret Service。

从早期 Windows Python 原型迁移：

```sh
migu auth import-legacy
```

只复制 `%LOCALAPPDATA%\MiguEmotionCLI\session.dpapi` 到独立的新加密存储，不删除原文件、不覆盖已有 Go 会话。迁移时请退出旧原型，不要让两套工具同时使用可能轮换的 refresh token；迁移后使用 Go CLI。

```sh
migu auth status       # 本地查看到期时间，不联网
migu auth ensure       # 按需续期
migu auth refresh      # 主动续期一次
migu auth logout       # 删除本 CLI 登录态，不退出浏览器、不撤销服务端授权
```

每次实际 API 操作都会检查有效期，距到期不足 1 小时或已到网页刷新时间时续期，并保存服务端返回的新 access/refresh token。文件锁保护多个 CLI 进程的续期和凭证写入。**不启动后台轮询、守护进程或定时任务**。Help、补全、dry-run 和 `auth status` 不会续期。

实测当前 access 和 refresh token 有效期约 12 小时。长时间不用 CLI、服务端撤销凭证或过期后仍需重新登录导入；按需续期不保证永久登录。浏览器和 CLI 登录态互不自动同步，平台的 token 轮换/复用策略尚无官方保证。

[返回 README](../README.md)。

