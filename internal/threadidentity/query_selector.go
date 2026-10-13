package threadidentity

import "regexp"

// QuerySelector resolves typed query coordinates using the existing grammar.
// An explicit numeric coordinate wins over a name's hyphen suffix: worker-3
// may be a literal comm when tid=11 was supplied. Explicit ID carriers must
// nevertheless agree. Identity retains its stricter display-only contract.
func QuerySelector(pid int, raw string) (int, string, bool) {
	if pid < 0 || pid > MaxPID {
		return pid, raw, false
	}
	sel := Parse(raw)
	name, explicit := sel.Name, false
	remaining := sel.Raw
	for _, re := range []*regexp.Regexp{threadSelectorPIDTokenRE, threadSelectorBracketRE} {
		for _, match := range re.FindAllStringSubmatch(remaining, -1) {
			value, valid := parsePositiveInt(match[1])
			if !valid || value > MaxPID || pid > 0 && value != pid {
				return pid, raw, false
			}
			pid, explicit = value, true
		}
		remaining = re.ReplaceAllString(remaining, " ")
	}
	if explicit {
		name = CleanName(remaining)
	} else if sel.HasPID {
		if pid > 0 && sel.HyphenPID && pid != sel.PID {
			return pid, CleanName(sel.Raw), true
		}
		if sel.PID > MaxPID || pid > 0 && pid != sel.PID {
			return pid, raw, false
		}
		pid = sel.PID
	}
	// A repeated matching display suffix is harmless. A different suffix is
	// part of the literal name, not another explicit PID/TID declaration.
	for pid > 0 && name != "" {
		nested := Parse(name)
		if !nested.HasPID || nested.PID != pid || nested.Name == name {
			break
		}
		name = nested.Name
	}
	return pid, name, true
}
