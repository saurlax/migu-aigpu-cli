# 开发与验证

```sh
gofmt -w cmd internal scripts
go vet ./...
go test -race ./...
go mod verify
go run ./scripts/package -version dev
```

测试覆盖自动续期、凭证轮换、续期失败保全、并发刷新锁、401 重试边界、重定向阻断、脱敏、DPAPI 存储、登录验证失败保全和无凭证 dry-run。设置 `MIGU_BROWSER_TEST` 为已安装 Chrome/Edge 的路径，可以运行使用本机浏览器及合成会话的隔离、提取与清理测试；无需真实账户。CI 在三个操作系统上测试；版本 tag 触发发布，附带六个平台压缩包和 SHA-256 校验文件。

## 项目结构

```text
cmd/migu/          CLI 入口
internal/cli/      命令、参数校验与输出
internal/api/      平台请求、续期与脱敏
internal/auth/     会话存储、刷新锁与手动导入
internal/login/    独立浏览器登录与会话验证
scripts/package/  多平台发布打包
```

## 发布

CI 和 Release 复用 [build.yml](../.github/workflows/build.yml)，运行格式检查、go vet、竞态测试和模块校验，再交叉构建 Linux、macOS、Windows 的 amd64 / arm64 压缩包。发布流程校验 checksums.txt 后上传同一轮构建产物，不在发布阶段重新构建。

推送 vMAJOR.MINOR.PATCH 格式的 tag 触发发布；带预发布后缀的 tag 创建 prerelease。维护者也可以在版本 tag 上手动运行 [Release](../.github/workflows/release.yml)。

[返回 README](../README.md)。
