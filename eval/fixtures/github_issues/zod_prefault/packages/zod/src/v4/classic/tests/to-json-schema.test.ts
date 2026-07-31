import { finalizeDefault } from "../../core/to-json-schema"

test("truthy prefault", () => {
  expect(finalizeDefault({ io: "input" }, { schema: { type: "string", _prefault: "x" } })).toEqual({
    type: "string",
    default: "x",
  })
})

test("existing default is not overwritten by prefault", () => {
  expect(
    finalizeDefault({
      io: "input",
    }, {
      schema: { type: "string", default: "existing", _prefault: "replacement" },
    }),
  ).toEqual({
    type: "string",
    default: "existing",
  })
})
