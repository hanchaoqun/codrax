import { finalizeDefault } from "../../core/to-json-schema"

test("truthy prefault", () => {
  expect(finalizeDefault({ io: "input" }, { schema: { type: "string", _prefault: "x" } })).toEqual({
    type: "string",
    default: "x",
  })
})
