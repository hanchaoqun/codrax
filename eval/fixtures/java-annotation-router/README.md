# java-annotation-router — annotation-driven routing + DI fixture

Blueprint shapes: spring-mvc @RequestMapping routing and field
injection, reduced to plain reflection (no framework dependency).

Design facts (cases assert exactly these):
- Handler (src/main/java/app/router/Handler.java) has EXACTLY THREE
  implementers: EchoHandler (/echo), UpperHandler (/upper),
  StatsHandler (/stats).
- Router.register scans the handler classes for @Route and maps
  route.path() -> instance; dispatch(path, body) looks the map up.
- StatsHandler's AuditLog field carries @Inject; Router.injectFields
  assigns the shared AuditLog instance reflectively before the
  handler is registered.
