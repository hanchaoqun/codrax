//go:build !windows

package outputdump

import (
	"fmt"
	"os"
	"reflect"
)

func explicitReportSingleLink(file *os.File) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	v := reflect.Indirect(reflect.ValueOf(info.Sys()))
	if v.IsValid() && v.Kind() == reflect.Struct {
		links := v.FieldByName("Nlink")
		if links.IsValid() && links.CanUint() && links.Uint() > 1 {
			return fmt.Errorf("atomic explicit report replacement does not support a target with multiple hard links")
		}
	}
	return nil
}
