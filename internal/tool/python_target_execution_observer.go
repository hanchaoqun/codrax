package tool

// Producer-owned Python instrumentation; kept in a Go build input so the
// existing clean-binary guard covers every executable observer byte.
const pythonTargetExecutionObserver = `# Observer setup is optional: failure cannot replace the probe's outcome.
import sys as _cx_setup_sys

class _CodraxNoTargetObserver:
    def finish(self):
        pass

_codrax_target_observer = _CodraxNoTargetObserver()
_cx_setup_trace = _cx_setup_sys.gettrace()
_cx_setup_profile = _cx_setup_sys.getprofile()
try:
    # Producer-owned runtime observation, not an adversarial Python sandbox.
    # Neither a planner status string nor a target filename alone proves execution.
    import ast as _cx_ast
    import base64 as _cx_base64
    import hashlib as _cx_hashlib
    import json as _cx_json
    import os as _cx_os
    import sys as _cx_sys
    import types as _cx_types
    import _thread as _cx_thread


    def _cx_code_hash(code):
        def constant(value):
            if isinstance(value, _cx_types.CodeType):
                return {"code": fields(value)}
            # marshal's reference flags can depend on object interning. Encode
            # immutable constant values explicitly so imported code and a fresh
            # compilation share an identity without dropping any constant.
            if value is None or value is Ellipsis:
                return {"type": type(value).__name__}
            if isinstance(value, (bool, int, str)):
                return {"type": type(value).__name__, "value": value}
            if isinstance(value, float):
                return {"type": "float", "value": value.hex()}
            if isinstance(value, complex):
                return {"type": "complex", "value": [value.real.hex(), value.imag.hex()]}
            if isinstance(value, bytes):
                return {"type": "bytes", "value": value.hex()}
            if isinstance(value, tuple):
                return {"type": "tuple", "value": [constant(x) for x in value]}
            if isinstance(value, frozenset):
                values = [constant(x) for x in value]
                values.sort(key=lambda x: _cx_json.dumps(x, sort_keys=True))
                return {"type": "frozenset", "value": values}
            raise ValueError("unsupported code constant")

        def fields(c):
            return {
                "name": c.co_name,
                "qualname": getattr(c, "co_qualname", ""),
                "filename": _cx_os.path.realpath(c.co_filename),
                "firstline": c.co_firstlineno,
                "args": [c.co_argcount, getattr(c, "co_posonlyargcount", 0), c.co_kwonlyargcount],
                "locals": c.co_nlocals,
                "stack": c.co_stacksize,
                "flags": c.co_flags,
                "code": c.co_code.hex(),
                "lines": getattr(c, "co_linetable", c.co_lnotab).hex(),
                "exceptions": getattr(c, "co_exceptiontable", b"").hex(),
                "names": c.co_names,
                "varnames": c.co_varnames,
                "freevars": c.co_freevars,
                "cellvars": c.co_cellvars,
                "constants": [constant(x) for x in c.co_consts],
            }

        data = _cx_json.dumps(fields(code), sort_keys=True, separators=(",", ":")).encode("utf-8")
        return _cx_hashlib.sha256(data).hexdigest()


    class _CodraxTargetObserver:
        def __init__(self):
            self.active = False
            self.failed = ""
            self.stopping = False
            self.events = 0
            self.observed = {}
            self.code_cache = {}
            self.filename_cache = {}
            self.source_paths = set()
            self.targets = []
            self.manifest = {}
            self.output = ""
            self.trace_hook = self.trace
            self.profile_hook = self.profile
            try:
                encoded = _cx_os.environ.get("CODRAX_TARGET_MANIFEST", "")
                if not encoded:
                    return
                self.manifest = _cx_json.loads(_cx_base64.b64decode(encoded))
                self.output = self.manifest["output_path"]
                if _cx_sys.gettrace() is not None or _cx_sys.getprofile() is not None:
                    self.failed = "existing_runtime_hook"
                    return
                for item in self.manifest["targets"]:
                    self.targets.append(self.prepare_target(item))
                    self.source_paths.add(_cx_os.path.realpath(_cx_os.path.join(self.manifest["execution_root"], item["path"])))
                self.thread_id = _cx_thread.get_ident()
                _cx_sys.addaudithook(self.audit)
                self.stopping = True
                _cx_sys.setprofile(self.profile_hook)
                _cx_sys.settrace(self.trace_hook)
                self.stopping = False
                self.active = True
            except BaseException:
                self.failed = "target_mapping_unavailable"

        def prepare_target(self, item):
            target = {"path": item["path"], "source_sha256": item["source_sha256"], "mapping_complete": False, "owners": []}
            full = _cx_os.path.join(self.manifest["execution_root"], item["path"])
            with open(full, "rb") as handle:
                source = handle.read(2097153)
            if len(source) > 2097152 or _cx_hashlib.sha256(source).hexdigest() != item["source_sha256"]:
                self.failed = "source_changed_before_execution"
                return target
            if not item["mapping_complete"] or not item["added_lines"]:
                return target
            tree = _cx_ast.parse(source, filename=full)
            compiled = compile(source, full, "exec", dont_inherit=True)
            inventory = []

            def collect_codes(c):
                inventory.append(c)
                for value in c.co_consts:
                    if isinstance(value, _cx_types.CodeType):
                        collect_codes(value)

            collect_codes(compiled)
            if len(inventory) > 4096:
                self.failed = "code_inventory_limit"
                return target
            owners = []
            unsupported = []

            def visit(node, depth=0):
                if isinstance(node, (_cx_ast.FunctionDef, _cx_ast.AsyncFunctionDef, _cx_ast.ClassDef)):
                    end = getattr(node, "end_lineno", None)
                    if not end:
                        raise ValueError("missing AST end line")
                    kind = "class" if isinstance(node, _cx_ast.ClassDef) else ("async_function" if isinstance(node, _cx_ast.AsyncFunctionDef) else "function")
                    first = min([node.lineno] + [x.lineno for x in node.decorator_list])
                    owners.append((node, depth, kind, first, end))
                if isinstance(node, (_cx_ast.Lambda, _cx_ast.ListComp, _cx_ast.SetComp, _cx_ast.DictComp, _cx_ast.GeneratorExp)):
                    unsupported.append((node.lineno, getattr(node, "end_lineno", node.lineno)))
                for child in _cx_ast.iter_child_nodes(node):
                    visit(child, depth + 1)

            visit(tree)
            selected = {}
            total_lines = len(source.splitlines())
            for line in item["added_lines"]:
                if line < 1 or line > total_lines or any(a <= line <= b for a, b in unsupported):
                    return target
                covering = [entry for entry in owners if entry[3] <= line <= entry[4]]
                if covering:
                    node, depth, kind, first, last = max(covering, key=lambda entry: entry[1])
                    candidates = [c for c in inventory if c.co_name == node.name and c.co_firstlineno == first]
                else:
                    kind, first, last = "module", 1, total_lines
                    candidates = [compiled]
                if len(candidates) != 1:
                    return target
                code = candidates[0]
                digest = _cx_code_hash(code)
                if digest not in selected:
                    selected[digest] = {"code_sha256": digest, "kind": kind, "first_line": first, "last_line": last, "changed_lines": [], "executed_lines": []}
                    self.observed[digest] = set()
                selected[digest]["changed_lines"].append(line)
            target["owners"] = sorted(selected.values(), key=lambda x: (x["first_line"], x["code_sha256"]))
            target["mapping_complete"] = bool(target["owners"])
            return target

        def audit(self, event, args):
            if self.stopping:
                return
            if event in ("sys.settrace", "sys.setprofile"):
                self.failed = "runtime_hook_changed"
            elif event in ("subprocess.Popen", "os.fork", "os.forkpty", "os.posix_spawn", "os.exec", "os.system"):
                self.failed = "unsupported_child_execution"

        def profile(self, frame, event, arg):
            # Python 3.13 Thread.start uses start_joinable_thread. Both runtime
            # builtin identities are unsupported, without relying on prose/name.
            starters = (_cx_thread.start_new_thread, getattr(_cx_thread, "start_joinable_thread", None))
            if event == "c_call" and any(arg is starter for starter in starters if starter is not None):
                self.failed = "unsupported_thread_execution"

        def trace(self, frame, event, arg):
            if self.failed or self.stopping:
                return self.trace_hook
            self.events += 1
            if self.events > 1000000:
                self.failed = "runtime_event_limit"
                return self.trace_hook
            if _cx_thread.get_ident() != self.thread_id:
                self.failed = "unsupported_thread_execution"
                return self.trace_hook
            if event != "line":
                return self.trace_hook
            code = frame.f_code
            # Filename is a cheap rejection only. Authority requires the full
            # current-source compiled code hash, including nested code/constants.
            filename = code.co_filename
            if filename not in self.filename_cache:
                if len(self.filename_cache) >= 4096:
                    self.failed = "runtime_filename_limit"
                    return self.trace_hook
                self.filename_cache[filename] = _cx_os.path.realpath(filename)
            if self.filename_cache[filename] not in self.source_paths:
                return self.trace_hook
            identity = id(code)
            if identity not in self.code_cache:
                if len(self.code_cache) >= 4096:
                    self.failed = "runtime_code_limit"
                    return self.trace_hook
                # Keep the object alive and key by identity: CodeType equality
                # is not the complete field-by-field code receipt above.
                try:
                    self.code_cache[identity] = (code, _cx_code_hash(code))
                except BaseException:
                    self.failed = "runtime_code_unknown"
                    return self.trace_hook
            digest = self.code_cache[identity][1]
            if digest in self.observed:
                lines = self.observed[digest]
                if len(lines) >= 8192:
                    self.failed = "runtime_line_limit"
                else:
                    lines.add(frame.f_lineno)
            return self.trace_hook

        def finish(self):
            if not self.output:
                return
            try:
                if self.active and (_cx_sys.gettrace() is not self.trace_hook or _cx_sys.getprofile() is not self.profile_hook):
                    self.failed = "runtime_hook_changed"
                self.stopping = True
                if self.active:
                    _cx_sys.settrace(None)
                    _cx_sys.setprofile(None)
                self.active = False
                for target in self.targets:
                    full = _cx_os.path.join(self.manifest["execution_root"], target["path"])
                    with open(full, "rb") as handle:
                        data = handle.read(2097153)
                    if _cx_hashlib.sha256(data).hexdigest() != target["source_sha256"]:
                        self.failed = "source_changed_after_execution"
                    for owner in target["owners"]:
                        owner["executed_lines"] = sorted(self.observed.get(owner["code_sha256"], set()))
                result = {"nonce": self.manifest["nonce"], "status": "unknown" if self.failed else "complete", "reason_code": self.failed, "targets": self.targets}
                with open(self.output, "w", encoding="utf-8") as handle:
                    _cx_json.dump(result, handle, sort_keys=True)
            except BaseException:
                # Missing output is unknown. Observation cannot replace or alter
                # the original probe's process result or exception handling.
                pass


    _codrax_target_observer = _CodraxTargetObserver()

    if _codrax_target_observer.failed:
        raise RuntimeError("target observer setup unavailable")
except BaseException:
    # Only restore the exact hooks present before our installation. An
    # observer which installed one hook before failing must not leave it on.
    try:
        _codrax_target_observer.stopping = True
        if _cx_setup_sys.gettrace() is not _cx_setup_trace:
            _cx_setup_sys.settrace(_cx_setup_trace)
        if _cx_setup_sys.getprofile() is not _cx_setup_profile:
            _cx_setup_sys.setprofile(_cx_setup_profile)
    except BaseException:
        pass
    _codrax_target_observer = _CodraxNoTargetObserver()
`
