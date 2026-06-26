logos version
logos status
logos help
logos roll --help
logos roll build -f examples/base-agent/Irollfile -t my-agent
logos page --help
logos page new --help
logos page new my-agent
logos page get

# 注：真正的 e2e 测试位于 iroll/e2e/（scenario_*_test.go），
# 辅助工具位于 iroll/e2e/testenv（Env.Build()、Env.DB()、Env.OpenWorkspace()、Env.CreatePage()）。
