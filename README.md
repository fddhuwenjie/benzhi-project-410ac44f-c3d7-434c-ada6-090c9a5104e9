# 射电干扰调查台

本项目是面向射电观测站频谱保障人员的版本化 HTTP JSON 服务。案件依次经过 `DETECTED`、`TRIAGED`、`PLANNED`、`EVIDENCE_COLLECTED`、`HYPOTHESIS_CONFIRMED`、`MITIGATED`、`VERIFIED` 和 `CLOSED`，并保留只追加审计轨迹、请求幂等结果与乐观并发修订号。

登记时会返回同一天线、重叠频段和相近观测时窗的相似候选，可通过 `association_disposition` 选择 `LINK` 或 `INDEPENDENT`。后续流程保存可解释的影响分项、稳定 `plan_item_id`、逐计划项三类证据覆盖、候选来源测试轮次、抑制尝试与独立复测，以及封存前五项材料清单。

案件集合 `GET /api/v1/interference-cases` 提供值班待办查询。默认排除 `CLOSED`，可组合使用 `state`、`severity`、`antenna_id`、`observation_start`、`observation_end` 和 `has_blockers`；`state` 与 `severity` 支持重复参数或逗号分隔值。响应按最近审计时间与案件标识确定排序，包含当前修订、下一动作、阻塞摘要、状态和严重度命中计数，以及绑定当前筛选条件的稳定 `cursor`。`page_size` 范围为 1 到 100。

## 构建与测试

```sh
go build ./cmd/server
go test ./...
```

## 运行

```sh
go run ./cmd/server -addr=127.0.0.1:19081 -data-dir=./data
```

未提供 `-addr` 时，服务读取 `PORT` 并绑定 `127.0.0.1:<PORT>`；若 `PORT` 也未设置，则使用 `127.0.0.1:19081`。客户端写请求必须提供 `X-Actor`、`X-Request-ID`，已有案件还应提供 `X-Expected-Revision`。

角色写为 `role:name`：影响研判使用 `duty`，候选来源操作使用 `investigator`，措施实施使用 `engineer`，复测和封存使用 `reviewer`。证据端点兼容原单条对象，并支持数组或 `{"evidence":[...]}` 批次；每个计划项的 `spectrum`、`device_reading` 和 `field_observation` 均齐备后才推进状态。

`PLANNED` 案件在尚无证据时，可由 `engineer` 在既有 `plan` 端点提交完整替代计划并填写 `replacement_reason`；新计划重新执行资源冲突校验，产生新的计划标识、计划项标识和递增计划修订。证据端点还接受 `{"action":"WITHDRAW","evidence_ids":[...],"correction_reason":"..."}`，用于在来源研判开始前原子撤回 1 到 50 条有效证据；撤回历史持续保留，原 `content_hash` 不得复用，覆盖不足时案件返回 `PLANNED`。

整改措施组成严格串行链：首次措施不填写 `previous_attempt_id`，后续措施必须引用最新失败尝试，待复测期间不能并行追加。复测结论一经记录不可改写；案件详情的 `mitigation_trend` 按链返回逐指标超限量、达标余量和相邻尝试改善幅度，无共同指标时明确标记不可比较。

案件详情在原案件字段之外返回 `progress`、同修订汇总与整改趋势。审计接口支持 `from_revision`、`event_type`、`page_size` 和 `cursor` 查询参数，响应包含 `events`、稳定下一页游标以及修订连续性和载荷摘要完整性状态。

## 全流程自检

```sh
go run ./cmd/server -selfcheck -addr=127.0.0.1:19081
```

自检使用临时数据目录启动真实回环 HTTP 服务，完成案件登记、研判、计划、取证、来源确认、抑制、复测和关闭后主动退出。
