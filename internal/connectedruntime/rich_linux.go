//go:build linux

package connectedruntime

import (
	"io"
	"os"
)

// CheckRichProcessesView retains the numeric role's boundaries and additionally
// verifies the fixed rich profile's private state. It does not prove host-wide
// filesystem/process visibility; the package acceptance gate proves visibility.
func CheckRichProcessesView(collector bool) error {
	if CheckView(collector) != nil {
		return ErrUnsafe
	}
	if !collector {
		return nil
	}
	return checkRichCollectorSecrets(func(path string) (io.Closer, error) { return os.Open(path) })
}

func checkRichCollectorSecrets(open func(string) (io.Closer, error)) error {
	for _, path := range []string{
		"/etc/home-lab-observer-connected-rich/credentials",
		"/var/lib/home-lab-observer-connected-rich/ledger",
		"/var/lib/home-lab-observer-connected-rich/enrollment",
		"/var/lib/home-lab-observer-connected-rich/uploader-root",
		"/var/lib/home-lab-observer-connected-rich/uploader-status",
	} {
		f, err := open(path)
		if err == nil {
			f.Close()
			return ErrUnsafe
		}
		if !os.IsPermission(err) && !os.IsNotExist(err) {
			return ErrUnsafe
		}
	}
	return nil
}
