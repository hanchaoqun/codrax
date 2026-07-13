//go:build !windows

package hitraceconv

import "os"

// publishConversionFileNoReplace atomically creates finalPath as another link
// to stagingPath. Link fails when finalPath already exists and therefore cannot
// overwrite a racing external owner. The caller removes the staging link only
// after creator identity and size have been registered and sealed.
func publishConversionFileNoReplace(stagingPath, finalPath string) (stagingMoved bool, err error) {
	return false, os.Link(stagingPath, finalPath)
}
