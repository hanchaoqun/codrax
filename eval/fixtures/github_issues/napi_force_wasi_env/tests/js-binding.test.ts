import { strict as assert } from 'assert'
import { renderNativeBinding } from '../cli/src/api/templates/js-binding'

function loaderSourceFor(value: string | undefined): string {
  const rendered = renderNativeBinding('sample')
  return rendered.replace(/process\.env\.NAPI_RS_FORCE_WASI/g, JSON.stringify(value))
}

assert.match(loaderSourceFor('true'), /true/)
assert.match(loaderSourceFor('error'), /error/)
assert.doesNotMatch(loaderSourceFor('false'), /false\)\s*\{/)
assert.doesNotMatch(loaderSourceFor('0'), /0\)\s*\{/)
