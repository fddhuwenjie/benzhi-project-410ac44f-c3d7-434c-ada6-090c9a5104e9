# BENZHI_README

## 项目说明
- 项目：benzhi-project-410ac44f-c3d7-434c-ada6-090c9a5104e9
- 项目用途：实现射电干扰案件从登记、研判、调查计划、证据采集、来源确认、抑制复测到审计关闭的完整 HTTP JSON 闭环。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 项目描述
- 项目名称：射电干扰调查台
- 项目介绍：面向射电观测站频谱保障人员的干扰事件调查 HTTP 服务，围绕一次干扰从发现登记、分级研判、调查设计、证据采集、来源确认、抑制验证到审计关闭的单一闭环推进，保留完整状态变更、版本冲突和操作轨迹。项目根目录必须提供简体中文 README.md，说明用途以及标准构建、运行和测试方式；目标形成不少于 20 个生产 Go 文件和约 2400 行真实生产 Go 代码。
- 项目概述：面向射电观测站频谱保障人员的干扰事件调查 HTTP 服务，围绕一次干扰从发现登记、分级研判、调查设计、证据采集、来源确认、抑制验证到审计关闭的单一闭环推进，保留完整状态变更、版本冲突和操作轨迹。项目根目录必须提供简体中文 README.md，说明用途以及标准构建、运行和测试方式；目标形成不少于 20 个生产 Go 文件和约 2400 行真实生产 Go 代码。
- 核心工作流：干扰事件由值班人员登记为 DETECTED，经特征去重与影响分级进入 TRIAGED；工程师编制可执行调查计划后进入 PLANNED，按计划提交带来源与时间窗的频谱证据并通过完整性检查进入 EVIDENCE_COLLECTED；调查人员登记来源假设和受控开关测试，满足判定规则后进入 HYPOTHESIS_CONFIRMED；实施抑制措施进入 MITIGATED，在独立复测满足阈值后进入 VERIFIED，最后由复核人员确认材料齐备并封存为 CLOSED。
- 对外接口：Go 服务提供版本化 HTTP JSON API，覆盖案件创建、查询和全部状态推进；写请求携带 request_id 与 expected_revision。服务支持 -addr=127.0.0.1:<port>，默认监听 127.0.0.1:19081，也可在未显式传入 -addr 时读取 PORT 并绑定 127.0.0.1:<PORT>，不得默认绑定 0.0.0.0、8080、80 或 3000。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/server -selfcheck -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-410ac44f-c3d7-434c-ada6-090c9a5104e9-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-410ac44f-c3d7-434c-ada6-090c9a5104e9-arm64 linux/arm64

docker run -it benzhi-project-410ac44f-c3d7-434c-ada6-090c9a5104e9-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/server -selfcheck -addr=127.0.0.1:19081`
