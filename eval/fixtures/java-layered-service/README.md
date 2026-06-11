# java-layered-service — 分层 Java 服务 fixture(spring-petclinic 形态缩减)

设计事实:
- 调用链:VisitController.create → VisitService.schedule → VisitRepository.insert → AuditLog.record(4 跳,跨 web/service/repo 三层)
- 配置优先级 3 层:ClinicConfig.DEFAULT_MAX_VISITS(代码默认 20)→ application.properties(clinic.max-visits=50)→ 环境变量 CLINIC_MAX_VISITS(运行时最高优先,resolveMaxVisits 内 getenv 覆盖)
