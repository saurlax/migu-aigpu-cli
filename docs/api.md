# 接口范围与验证记录

以下记录来自原有 README，保留的是项目开发时的验证结果，不代表当前平台的持续可用性保证。接口来自平台网页前端及本人账户的只读实测。

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

[返回 README](../README.md)。
