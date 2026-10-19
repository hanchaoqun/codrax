package tool

import (
	"github.com/hanchaoqun/codrax/internal/immutablefile"
	"os"
)

const immutableBlobPublicationSupported = immutablefile.PublicationSupported
const immutableBlobReadFlags = immutablefile.ReadFlags

func immutableBlobRenameNoReplace(parent *os.File, dir, stage, leaf string) error {
	return immutablefile.RenameNoReplace(parent, dir, stage, leaf)
}
