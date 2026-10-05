# migu-aigpu-cli

咪咕仝学 AIGPU 平台的非官方 Go CLI。参考 [autodl-cli](https://github.com/saurlax/autodl-cli) 的 Cobra 命令结构，支持嵌套命令、Shell 补全、JSON 输出和多平台二进制。

接口来自平台网页前端及本人账户的只读实测，**不是赛事官方 SDK**。平台升级可能改变接口。默认不创建、释放实例，不保存镜像、不提交赛事评测、不操作支付。

## 安装

在 [Releases](https://github.com/saurlax/migu-aigpu-cli/releases) 下载适合系统的压缩包，核对 `checksums.txt`，解压后把 `migu` / `migu.exe` 放到 PATH。支持 Windows、Linux、macOS 的 amd64 / arm64。

Go 1.26 及以上也可以直接安装：

```sh
go install github.com/saurlax/migu-aigpu-cli/cmd/migu@latest
# 或在项目目录
go build -o migu ./cmd/migu
```

## 登录

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

## 使用

```sh
migu instance list
migu instance list --json --page 1 --page-size 20
migu instance get INSTANCE_UUID
migu instance status INSTANCE_UUID
migu instance metrics INSTANCE_UUID
migu image list
migu image list --private --idc IDC_ID
migu dataset list
migu storage list --idc IDC_ID
```

实例列表默认表格；详情、镜像目录、数据集和存储保留平台 JSON 结构。`--json` 返回完整的**已脱敏**成功响应。包括嵌套环境变量、密码、Notebook token、accessKey、带签名链接在内的已识别凭证都被遮蔽，无原始凭证输出选项。

```sh
# 默认预览，不联网、不改动 GPU
migu instance start INSTANCE_UUID --mode GPU
migu instance stop INSTANCE_UUID
# 明确发送平台请求（可能影响计费）
migu instance start INSTANCE_UUID --execute
migu instance start INSTANCE_UUID --mode CPU --execute
migu instance stop INSTANCE_UUID --execute
# 任何 API 命令都可强制预览；优先于 --execute
migu instance start INSTANCE_UUID --execute --dry-run
```

开关机成功响应表示平台受理请求，CLI 随后查询一次并显示当前状态，不将受理结果当成已完成。需要使用 `instance status` 确认最终状态。开关机端点和参数已在网页源码确认，本项目开发验收**没有实际启动或关闭 GPU**。

只读查询遇到 HTTP 401 时刷新并重试一次；开关机等变更请求不自动重放，网络报错后请先查询实例状态。跨域重定向被禁止，API Origin 固定，避免凭证发往其他站点。

全局参数：`--json`、`--dry-run`、`--team ID`、`--timeout 60s`、`--version`。默认 HTTP 超时 60 秒，可用 `--timeout 2m` 延长。退出码：0 成功/帮助；1 参数、认证、网络或平台错误。Shell 补全：

```sh
migu completion bash
migu completion zsh
migu completion fish
migu completion powershell
```

## 接口范围

| 命令 | 方法与路径 | 验证 |
| --- | --- | --- |
| 自动续期 / `auth refresh` | POST `/v4/oauth/token` | 已实测 |
| `instance list` | POST `/v1/cloud/traininfer/v1/instance/search` | 已实测 |
| `instance get/status` | GET `/v1/cloud/traininfer/v1/instance/{id}` | 已实测 |
| `instance metrics` | POST `/v1/cloud/traininfer/v1/instance/metric/batchQryNowMetricsByInstanceUuidList` | 已实测 |
| `instance start/stop` | PUT `/v1/cloud/traininfer/v1/instance/{id}/start` 或 `/stop` | 源码确认、仅预览验收 |
| `image list` | POST `/v1/cloud/traininfer/mirrorversion/list` | 已实测 |
| `dataset list` | GET `/v1/cloud/traininfer/dataset/list` | 已实测 |
| `storage list` | POST `/v1/cloud/traininfer/filestorage/list` | 已实测 |

JSON 查询沿用平台分页及镜像层级，不承诺所有静态发现端点都可用。后续可以根据实测逐项扩展。

## 开发

```sh
go test ./...
go vet ./...
go run ./scripts/package -version v0.1.0
```

测试覆盖自动续期、凭证轮换、续期失败保全、并发刷新锁、401 重试边界、重定向阻断、脱敏、DPAPI 存储、登录验证失败保全和无凭证 dry-run。设置 `MIGU_BROWSER_TEST` 为已安装 Chrome/Edge 的路径，可以运行使用本机浏览器及合成会话的隔离、提取与清理测试；无需真实账户。CI 在三个操作系统上测试；版本 tag 触发发布，附带六个平台压缩包和 SHA-256 校验文件。

MIT License。发布打包脚本参考 AutoDL CLI，并保留其版权声明。
