export function renderNativeBinding(localName: string): string {
  return `
let nativeBinding = null
let nativeBindingError = null
let wasiBinding = null
let wasiBindingError = null

try {
  nativeBinding = require('./${localName}.node')
} catch (err) {
  nativeBindingError = err
}

if (!nativeBinding || process.env.NAPI_RS_FORCE_WASI) {
  try {
    wasiBinding = require('./${localName}.wasi.cjs')
  } catch (err) {
    wasiBindingError = err
  }

  if (wasiBinding) {
    nativeBinding = wasiBinding
  }
}

if (!nativeBinding) {
  if (process.env.NAPI_RS_FORCE_WASI === 'error' && wasiBindingError) {
    throw wasiBindingError
  }
  throw nativeBindingError || wasiBindingError
}

module.exports = nativeBinding
`
}
