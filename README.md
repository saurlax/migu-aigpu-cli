# Migu AIGPU CLI

[![CI](https://github.com/saurlax/migu-aigpu-cli/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/saurlax/migu-aigpu-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/saurlax/migu-aigpu-cli)](https://github.com/saurlax/migu-aigpu-cli/releases)
[![Go](https://img.shields.io/badge/go-1.26%2B-00ADD8)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**在终端中查询和管理咪咕仝学 AIGPU 实例。**

Migu AIGPU CLI 是面向[咪咕仝学 AIGPU 平台](https://aigpu.migu.cn)的开源、非官方命令行客户端。通过 `migu` 查询实例、运行指标、镜像、数据集和存储，使用独立浏览器完成登录，并为脚本提供脱敏 JSON 输出。

> **项目状态：** 已实现浏览器登录、加密会话存储、按需续期和资源查询；现有实例开关机默认预览，需加 `--execute` 才会发送请求。接口来自平台网页前端和本人账户的只读实测，平台升级可能改变接口。本项目不是官方 SDK，也不提供实例创建、释放、镜像保存、赛事评测提交或支付功能。

## 主要功能

- **查询资源。** 查看实例列表、详情、状态、当前指标，以及公共或私有镜像、数据集和存储。
- **浏览器登录。** 在独立 Chrome / Edge 窗口中正常登录，自动验证并保存必要会话。
- **管理现有实例。** 预览开关机请求，明确指定 `--execute` 后启动 GPU / CPU 模式或关机。
- **接入脚本。** 使用脱敏 JSON、分页、团队选择、请求超时和 Shell 补全。

## 安装

从 [GitHub Releases](https://github.com/saurlax/migu-aigpu-cli/releases) 下载压缩包，核对 `checksums.txt`，解压后将 `migu`（Windows 为 `migu.exe`）放到 `PATH`。

| 操作系统 | 架构 |
| --- | --- |
| Linux | amd64、arm64 |
| macOS | amd64、arm64 |
| Windows | amd64、arm64 |

使用 **Go 1.26 及以上版本**也可以从源码安装：

```sh
go install github.com/saurlax/migu-aigpu-cli/cmd/migu@latest
```

确保 Go 的二进制安装目录已加入 `PATH`，然后运行 `migu --version`。

## 快速上手

需要本机已有 Chrome、Edge 或兼容 Chromium 浏览器，以及可用的桌面环境。

```sh
migu auth login
# 在打开的独立浏览器窗口中完成平台登录。
migu auth status
migu instance list
migu image list
```

CLI 使用一次只读实例查询验证会话后保存凭证，关闭专用浏览器并删除临时配置。取消、超时或验证失败会保留原有 CLI 会话。自动检测不到浏览器时，可以指定可执行文件路径：

```sh
migu auth login --browser "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
migu auth login --wait 15m
```

Windows 使用当前用户 DPAPI 加密存储；macOS 使用 Keychain，Linux 使用 Secret Service。密钥环失败不会降级到明文存储。实际 API 操作按需续期，不运行后台任务；会话过期或被撤销后需要重新登录。

详见[登录与会话管理](docs/authentication.md)，包含手动导入、存储位置、续期规则和旧版迁移。Windows Edge 登录流程已完成真实验收；其他系统已通过编译和单元测试，仍需真实桌面登录验收。

## 使用

运行 `migu --help` 或 `migu instance --help` 查看命令，具体选项通过 `COMMAND --help` 查看。将以下 `INSTANCE_UUID` 替换为实例列表中的 UUID；将 `IDC_ID` 替换为平台数据中心 ID。

```sh
migu instance list --page 1 --page-size 20
migu instance get INSTANCE_UUID
migu instance status INSTANCE_UUID
migu instance metrics INSTANCE_UUID
migu image list --private --idc IDC_ID
migu dataset list
migu storage list --idc IDC_ID
```

实例列表默认显示表格，状态默认显示状态值；详情、镜像目录、数据集和存储保留平台 JSON 结构。输出会遮蔽已识别的凭证字段，包括嵌套环境变量、密码、Notebook token、accessKey 和带签名链接，没有原始凭证输出选项。

### 开关机

开关机命令**默认只预览**请求，不联网、不改动实例：

```sh
migu instance start INSTANCE_UUID --mode GPU
migu instance stop INSTANCE_UUID
```

确认后加 `--execute` 发送平台请求，可能影响计费：

```sh
migu instance start INSTANCE_UUID --mode GPU --execute
migu instance start INSTANCE_UUID --mode CPU --execute
migu instance stop INSTANCE_UUID --execute
```

`--dry-run` 优先于 `--execute`，也可用于预览其他 API 请求：

```sh
migu instance start INSTANCE_UUID --execute --dry-run
migu instance list --dry-run
```

开关机成功响应表示平台受理请求。CLI 随后查询一次当前状态，最终结果仍需通过 `instance status` 确认。开关机端点和参数已在网页源码中确认，项目开发验收仅做了预览，没有实际启动或关闭 GPU。

### 命令索引

| 命令 | 用途 |
| --- | --- |
| `auth login` | 独立浏览器登录、验证并保存会话 |
| `auth capture` | 从已登录的浏览器手动导入会话 |
| `auth status` | 本地查看登录态和到期时间 |
| `auth ensure` / `auth refresh` | 按需续期 / 主动续期一次 |
| `auth logout` | 删除本 CLI 的本地会话 |
| `auth import-legacy` | 迁移 Windows Python 原型的会话 |
| `instance list` | 分页查询实例，可按数据中心和状态筛选 |
| `instance get INSTANCE_UUID` | 查询脱敏详情 |
| `instance status INSTANCE_UUID` | 查询生命周期状态 |
| `instance metrics INSTANCE_UUID` | 查询当前运行指标 |
| `instance start INSTANCE_UUID` | 预览开机；加 `--execute` 执行 |
| `instance stop INSTANCE_UUID` | 预览关机；加 `--execute` 执行 |
| `image list` | 查询公共镜像；加 `--private` 查询私有镜像 |
| `dataset list` | 分页查询数据集 |
| `storage list --idc IDC_ID` | 查询指定数据中心的存储 |
| `completion bash\|zsh\|fish\|powershell` | 生成 Shell 补全脚本 |

## 自动化

```sh
migu instance list --json --page 1 --page-size 20
migu instance status INSTANCE_UUID --json --timeout 2m
migu instance list --team TEAM_ID --json
migu completion powershell
```

| 全局参数 | 用途 |
| --- | --- |
| `--json` | 输出完整、已脱敏的成功响应 |
| `--dry-run` | 预览 API 请求，无需认证或网络访问 |
| `--team ID` | 显式指定平台团队 ID |
| `--timeout DURATION` | 每次 HTTP 请求的超时，默认 `60s` |
| `--help` / `--version` | 查看帮助 / 版本 |

退出码 `0` 表示成功或帮助，`1` 表示参数、认证、网络或平台错误。自动登录的进度写入 stderr，成功会话元数据写入 stdout，不输出凭证。

只读查询遇到 HTTP 401 时刷新并重试一次；开关机等变更请求不会自动重放。网络报错后应先查询实例状态，再决定是否重试。API Origin 固定，跨域重定向被禁止。JSON 查询沿用平台分页和镜像层级；具体端点和验证范围见[接口说明](docs/api.md)。

## 开发

```sh
git clone https://github.com/saurlax/migu-aigpu-cli.git
cd migu-aigpu-cli
go build -o migu ./cmd/migu
go vet ./...
go test -race ./...
go mod verify
```

单元测试使用本地服务、合成会话和临时存储，无需真实账户或 GPU。CI 在 Linux、macOS、Windows 上运行检查，并构建六个平台的发布压缩包和 SHA-256 校验文件。详见[开发与验证](docs/development.md)，包含项目结构、可选浏览器测试和发布流程。

## 参与贡献

欢迎提交问题报告、文档改进和经过验证的接口支持。提交前先搜索 [GitHub Issues](https://github.com/saurlax/migu-aigpu-cli/issues)。报告请附上 CLI 版本、操作系统、命令和脱敏错误信息，移除浏览器会话、token 和其他凭证。

新增平台接口前请先讨论调用方式与验证范围，区分网页源码确认、只读实测和实际变更验收。提交 Pull Request 前运行上述开发检查，并同步更新相关文档。

## 许可与致谢

本项目采用 [MIT License](LICENSE)。命令结构使用 [Cobra](https://github.com/spf13/cobra)，参考 [AutoDL CLI](https://github.com/saurlax/autodl-cli)；发布打包脚本沿用其实现并保留版权声明。
