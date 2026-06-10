# Logos 待办

核心理念：工具本身不是护城河，.iroll 的流通才是。优先级围绕「让好的 .iroll 能被创造和分享」排列。

## 第一阶段：冻结协议

- [ ] 冻结 .iroll 格式 — 表结构、context 格式、Resources 目录规范确定后不再轻易变更
- [ ] 补齐 loop CLI — DB 层已完成（seed CRUD + run start/update/complete/abort/reflect/history），CLI 命令待补齐
- [x] memory 重构 — 表结构增加 name/question/page_id 列（参考 dna Q&A），去掉 add-memory CLI（memory 由 context 压缩自动插入），补齐 query-memory CLI
- [ ] forget 表实现 — 遗忘机制，sleep 循环的落地（未开始）

## 第二阶段：让人「哇」的包

做出让人拿起来就想分享的 .iroll，用户买的是好 agent，不是好工具。

- [ ] cat-agent — 傲娇猫人格（展示 dna 机制）
- [ ] tutor — 严谨家教（展示 dna + book 注入教材）
- [ ] code-reviewer — 代码审查员（展示 skill 能力）
- [ ] pm — 产品经理（展示综合能力）

## 第三阶段：让流通发生

irollhub 是产品本身，CLI 只是运行时。

- [x] irollhub 最小版本 — 上传、下载、列表、搜索（14 个 task 全部完成）
- [x] .iroll 包版本管理 — 语义化版本校验、重复检测、checksum
- [x] 包发现机制 — FTS5 搜索、标签、下载量统计
- [ ] logos CLI 接入 irollhub — login/logout、push、pull、search 命令

## 第四阶段：体验优化

- [ ] engine 机制 — 驱动 loop 自动调度
- [ ] context 压缩策略 — 溢出时的摘要算法
- [ ] memory 页面隔离 — 确认 memory 按 page_id 隔离
- [ ] 前端界面 — 可视化管理 iroll 包、页面、记忆



第一阶段：冻结协议（1-2 周）
任务	时间	原因
冻结 .iroll 格式	1 天	主要是决策，不是代码
loop CLI	3-5 天	db 层已完成，只需 cmd 入口 + 测试
memory query	2-3 天	单表查询 + CLI 入口，不复杂
forget 表	2-3 天	新表 + sleep 逻辑
第二阶段：让人「哇」的包（1 周）
任务	时间	原因
4 个 demo .iroll	5-7 天	每个 1-2 天：调教 dna、写 content、准备 book 数据
这部分看似简单，但调教出一个真正让人「哇」的 agent 比写代码难。dna 的问答设计、人格的一致性、能力的实用性 — 这些需要反复测试。

第三阶段：让流通发生（已完成 irollhub，剩余 CLI 接入 1 周）
任务	时间	原因
irollhub 最小版	已完成	上传/下载 API、FTS5 搜索、OAuth、MinIO 存储、15 个 commit
logos CLI 接入	3-5 天	login/logout/push/pull/search 命令，对接 irollhub API
第四阶段：体验优化（3-4 周）
任务	时间	原因
engine 机制	1-2 周	架构级变更
context 压缩	3-5 天	需要实验和调参
memory 隔离	2-3 天	schema + 迁移
前端界面	2-3 周	独立的前端项目
总计
阶段	时间	里程碑
第一阶段	1-2 周	工具可用，可以开源
第二阶段	1 周	有东西可以展示
第三阶段	已完成 irollhub + 1 周 CLI	流通闭环，生态起步
第四阶段	3-4 周	体验打磨
合计	~1.5 个月（已大幅缩短）	
建议节奏：

第 2 周：发布 v0.1（loop CLI + memory query 完成）→ 开源
第 3 周：发布 demo .iroll 包 → 发帖宣传
第 4 周：logos CLI 接入 irollhub → 流通发生
第 5 周起：根据用户反馈决定做什么
先跑起来，边跑边补。等全做完再推就晚了。
