package tracewire

import "strings"

// ResourceOperationDescriptor uses the closed operation registry, never text
// from the user's question. Size units are asserted only for the original
// malloc/mmap protocol; extended producer families retain unspecified units.
func ResourceOperationDescriptor(operation string) (family, action, sizeUnit string, ok bool) {
	normalized, counter, ok := NormalizeResourceOperation(operation)
	if !ok || normalized != operation {
		return "", "", "", false
	}
	sizeUnit = "unspecified"
	switch operation {
	case "AllocEvent":
		return "heap", "allocate", "bytes", true
	case "FreeEvent":
		return "heap", "release", "bytes", true
	case "MmapEvent":
		return "mapping", "allocate", "bytes", true
	case "MunmapEvent":
		return "mapping", "release", "bytes", true
	case "FD_Open_Event":
		return "FD", "open", sizeUnit, true
	case "FD_Close_Event":
		return "FD", "close", sizeUnit, true
	case "THREAD_Create_Event":
		return "THREAD", "create", sizeUnit, true
	case "THREAD_Destroy_Event":
		return "THREAD", "destroy", sizeUnit, true
	}
	family = strings.TrimSuffix(strings.TrimPrefix(counter, "NativeHook_"), "_Active")
	action = "allocate"
	if strings.HasSuffix(operation, "_Free_Event") {
		action = "release"
	}
	return family, action, sizeUnit, true
}
