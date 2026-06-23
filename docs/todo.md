# Logos 待办

核心理念：工具本身不是护城河，.iroll 的流通才是。优先级围绕「让好的 .iroll 能被创造和分享」排列。

## 第一阶段：冻结协议

- [x] 冻结 .iroll 格式 — 表结构、context 格式、Resources 目录规范确定后不再轻易变更
- [x] 补齐 loop CLI — seed CRUD、run 生命周期、动态 context、页面删除清理均已完成
- [x] memory 重构 — 表结构增加 name/question/page_id 列（参考 dna Q&A），去掉 add-memory CLI，为未来 context 压缩写入预留 DB API，补齐 query-memory CLI
- [x] memory 页面隔离 — memory 按 `page_id` 写入和查询
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
- [x] logos CLI 接入 irollhub — login/logout、push、pull、search 命令

## 第四阶段：体验优化

- [ ] Agent Loop 使用协议 — 明确 agent 如何从 context 自主选择、更新和结束 loop；Logos 不负责调度
- [ ] context 压缩策略 — 溢出时的摘要算法
- [x] 前端界面 — irollhub-web React 前端（浏览、搜索、包详情）

## 当前建议顺序

1. 冻结 `.iroll` v1 协议，并增加明确的 schema version 校验。
2. 制作并真实使用一个 demo `.iroll`，优先验证 dna、loop、memory、book、skill 的组合体验。
3. ~~将 Logos CLI 接入 irollhub，完成包的发现、下载和发布闭环。~~ ✅ 已完成
4. 根据真实使用反馈再决定 forget、context 压缩和前端的优先级。

暂不在 Logos 内实现 loop 自动调度。Agent 自主决定做什么，Logos 只管理上下文和生命记录。


meeting？