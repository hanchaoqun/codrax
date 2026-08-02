import { strict as assert } from 'assert'
import { renderNativeBinding } from '../cli/src/api/templates/js-binding'

function forceWasiWithNative(value: string | undefined): boolean {
  const rendered = renderNativeBinding('sample')
  const branch = rendered.match(/if\s*\(\s*(?<condition>!nativeBinding[^)]*)\)\s*\{/)
  assert.ok(branch?.groups?.condition, 'generated loader must expose its WASI branch condition')

  const literal = value === undefined ? 'undefined' : JSON.stringify(value)
  const condition = branch.groups.condition.replace(
    /process\.env\.NAPI_RS_FORCE_WASI/g,
    literal,
  )
  return Function('nativeBinding', `return Boolean(${condition})`)(true)
}

assert.equal(forceWasiWithNative('true'), true)
assert.equal(forceWasiWithNative('error'), true)
assert.equal(forceWasiWithNative('false'), false)
assert.equal(forceWasiWithNative('0'), false)
assert.equal(forceWasiWithNative(''), false)
assert.equal(forceWasiWithNative(undefined), false)
