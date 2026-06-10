type JSONSchema = {
  default?: unknown
  _prefault?: unknown
  type?: string
}

type Result = {
  schema: JSONSchema
}

type Context = {
  io: "input" | "output"
}

export function finalizeDefault(ctx: Context, result: Result): JSONSchema {
  if (ctx.io === "input" && result.schema._prefault) result.schema.default ??= result.schema._prefault
  delete result.schema._prefault
  return result.schema
}
