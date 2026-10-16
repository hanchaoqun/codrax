//go:build windows

package outputdump

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func explicitReportSingleLink(file *os.File) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &info); err != nil {
		return err
	}
	if info.NumberOfLinks > 1 {
		return fmt.Errorf("atomic explicit report replacement does not support a target with multiple hard links")
	}
	return nil
}
